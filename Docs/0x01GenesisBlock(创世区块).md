# 0x01GenesisBlock(创世区块)

上次讲了[以太坊在mac下的本地编译环境](https://www.jianshu.com/p/49f8d83978d8)，从这次开始我们从创世区块入手来逐步研读以太坊核心的部分源代码。

# 创世命令
> geth --datadir data0 init genesis.json

- geth main         -->在/cmd/geth/main.go
- --datadir data0  -->启动创世命令后创建的专门储存节点数据的文件目录
- init     -->调用的是cmd/geth/chaincmd.go中的initCommand, initCommand调用的是initGenesis(ctx *cli.Context)
- genesis.json -->创世区块配置文件，与创世区块的数据结构一一对应

# 废话少说读代码

### Genesis struct

首先，在我们还对源码结构不太熟悉的情况下，通过全局搜索来寻找关于创世区块的结构定义。

![找啊找啊找朋友](https://upload-images.jianshu.io/upload_images/830585-a9a47d62378f71ac.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

```
// 创世区块类结构，后面的是区块对应json字段名称
type Genesis struct {
	// 配置文件，用于指定链的chainId(network id)
	Config     *params.ChainConfig `json:"config"`
	// 随机数，与Mixhash组合用于满足POW算法要求
	Nonce      uint64              `json:"nonce"`
	// 时间戳
	Timestamp  uint64              `json:"timestamp"`
	// 区块额外信息
	ExtraData  []byte              `json:"extraData"`
	// Gas消耗量限制
	GasLimit   uint64              `json:"gasLimit"   gencodec:"required"`
	// 区块难度值
	Difficulty *big.Int            `json:"difficulty" gencodec:"required"`
	// 由上个区块的一部分生成的Hash，和Nonce组合用于找到满足POW算法的条件
	Mixhash    common.Hash         `json:"mixHash"`
	// 矿工地址
	Coinbase   common.Address      `json:"coinbase"`
	// 创世区块初始状态
	Alloc      GenesisAlloc        `json:"alloc"      gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	/** 下面字段用于共识测试，不要在创世区块中使用
	*/
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	// 父区块哈希
	ParentHash common.Hash `json:"parentHash"`
}
```

### SetupGenesisBlock

了解了创世区块的数据结构后，我们来看一下以太坊是如何启动创世区块的。

```
/**如果存储的区块链配置不兼容，将调用该方法更新。为了避免冲突，错误将会被作为参数返回，并且新的配置
和原有配置都会被返回。
*/
func SetupGenesisBlock(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		return params.AllEthashProtocolChanges, common.Hash{}, errGenesisNoConfig
	}

	// Just commit the new block if there is no stored genesis block.
	// 如果没有存储的genesis块，只需提交新块。
	stored := rawdb.ReadCanonicalHash(db, 0)	// 从数据库中获取genesis对应的区块
	if (stored == common.Hash{}) {
		if genesis == nil {
			// genesis和stored都为空，使用主网
			log.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
		} else {
			// 使用自定义的genesis配置
			log.Info("Writing custom genesis block")
		}
		// 创世区块写入数据库
		block, err := genesis.Commit(db)
		return genesis.Config, block.Hash(), err
	}

	// Check whether the genesis block is already written.
	// 检查创世区块是否写入成功
	if genesis != nil {
		// 获取创世区块哈希
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}

	// Get the existing chain configuration.
	// 获取当前区块链相关配置
	newcfg := genesis.configOrDefault(stored)
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		// 读取失败，说明创世区块写入被中断
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return newcfg, stored, nil
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	// 特殊情况：如果没有新的配置，请勿更改非主网链的相关配置
	// 这些链会得到AllProtocolChanges以及compat error，如果我们继续
	if genesis == nil && stored != params.MainnetGenesisHash {
		return storedcfg, stored, nil
	}

	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	// 检查配置兼容性并写入配置
	// 兼容性错误将返回给调用者，除非我们已经处于块0
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	// 如果compatErr为空并且非高度为0的区块，那么久不能更改genesis配置了
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)
	return newcfg, stored, nil
}
```

### ToBlock

ToBlock方法使用genesis的数据，使用基于内存的数据库，然后创建了一个block并返回。

```
// ToBlock, 这个方法使用genesis的数据，使用基于内存的数据库，然后创建了一个block并返回。
func (g *Genesis) ToBlock(db ethdb.Database) *types.Block {
	if db == nil {
		db = ethdb.NewMemDatabase()
	}
	// 用genesis的数据给statedb赋值
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
	for addr, account := range g.Alloc {
		statedb.AddBalance(addr, account.Balance)
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}
	root := statedb.IntermediateRoot(false)
	// 填充head值
	head := &types.Header{
		Number:     new(big.Int).SetUint64(g.Number),
		Nonce:      types.EncodeNonce(g.Nonce),
		Time:       new(big.Int).SetUint64(g.Timestamp),
		ParentHash: g.ParentHash,
		Extra:      g.ExtraData,
		GasLimit:   g.GasLimit,
		GasUsed:    g.GasUsed,
		Difficulty: g.Difficulty,
		MixDigest:  g.Mixhash,
		Coinbase:   g.Coinbase,
		Root:       root,
	}
	if g.GasLimit == 0 {
		head.GasLimit = params.GenesisGasLimit
	}
	if g.Difficulty == nil {
		head.Difficulty = params.GenesisDifficulty
	}

	// 数据库提交
	statedb.Commit(false)
	statedb.Database().TrieDB().Commit(root, true)

	return types.NewBlock(head, nil, nil, nil)
}
```

### Commit

上面两个主要函数都用到了Commit方法，接下来我们就看下该方法到底做了哪些操作。

```
// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
// Commit方法调用rawdb.WriteChainConfig(db, block.Hash(), config)函数把给定的genesis的block
// 和state写入数据库，该block被认为是规范的区块链头
func (g *Genesis) Commit(db ethdb.Database) (*types.Block, error) {
	
	// 取到genesis对应的block
	block := g.ToBlock(db)
	if block.Number().Sign() != 0 {
		return nil, fmt.Errorf("can't commit genesis block with number > 0")
	}
	// 写入总难度
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), g.Difficulty)
	// 写入区块
	rawdb.WriteBlock(db, block)
	// 写入区块数据
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	// 写入 headerPrefix + num (uint64 big endian) + numSuffix -> hash
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())

	config := g.Config
	if config == nil {
		config = params.AllEthashProtocolChanges
	}
	// 写入 ethereum-config-hash -> config
	rawdb.WriteChainConfig(db, block.Hash(), config)
	return block, nil
}
```

### 各种模式的GenesisBlock

```

// 各种返回模式的Genesis
// GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
// GenesisBlockForTesting创建并写入一个块，其中addr具有给定的wei余额。
func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.Block {
	g := Genesis{Alloc: GenesisAlloc{addr: {Balance: balance}}}
	return g.MustCommit(db)
}

// DefaultGenesisBlock returns the Ethereum main net genesis block.
// DefaultGenesisBlock返回以太坊主网络创世块
func DefaultGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.MainnetChainConfig,
		Nonce:      66,
		ExtraData:  hexutil.MustDecode("0x11bbe8db4e347b4e8c937c1c8370e4b5ed33adb3db69cbdb7a38e1e50b1b82fa"),
		GasLimit:   5000,
		Difficulty: big.NewInt(17179869184),
		Alloc:      decodePrealloc(mainnetAllocData),
	}
}

// DefaultTestnetGenesisBlock returns the Ropsten network genesis block.
// DefaultTestnetGenesisBlock返回Ropsten网络创世块。
func DefaultTestnetGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.TestnetChainConfig,
		Nonce:      66,
		ExtraData:  hexutil.MustDecode("0x3535353535353535353535353535353535353535353535353535353535353535"),
		GasLimit:   16777216,
		Difficulty: big.NewInt(1048576),
		Alloc:      decodePrealloc(testnetAllocData),
	}
}

// DefaultRinkebyGenesisBlock returns the Rinkeby network genesis block.
// DefaultRinkebyGenesisBlock返回Rinkeby网络创世块。
func DefaultRinkebyGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.RinkebyChainConfig,
		Timestamp:  1492009146,
		ExtraData:  hexutil.MustDecode("0x52657370656374206d7920617574686f7269746168207e452e436172746d616e42eb768f2244c8811c63729a21a3569731535f067ffc57839b00206d1ad20c69a1981b489f772031b279182d99e65703f0076e4812653aab85fca0f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   4700000,
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(rinkebyAllocData),
	}
}

// DeveloperGenesisBlock returns the 'geth --dev' genesis block. Note, this must
// be seeded with the
// eveloperGenesisBlock返回'geth --dev'创世块。
func DeveloperGenesisBlock(period uint64, faucet common.Address) *Genesis {
	// Override the default period to the user requested one
	config := *params.AllCliqueProtocolChanges
	config.Clique.Period = period

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	return &Genesis{
		Config:     &config,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, 65)...),
		GasLimit:   6283185,
		Difficulty: big.NewInt(1),
		Alloc: map[common.Address]GenesisAccount{
			common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
			common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
			common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
			common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
			common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
			common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
			common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
			common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
			faucet: {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		},
	}
}
```



.
.
.
.
>###互联网颠覆世界，区块链颠覆互联网!

>###### --------------------------------------------------20180904 23:11

