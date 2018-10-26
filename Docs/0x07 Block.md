# 以太坊源码研读0x07 Block

前面看了以太坊的交易模块，而交易都是要打包在区块上的。Block是Eth上存储价值信息的核心数据结构之一。

一个完整的Block大概包括以下几部分：

- 1.所有账户的相关活动，都是以Transaction格式存储，每个Block有一个Tx的列表

- 2.每个交易的执行结果，由一个Receipt对象与其包含的一组Log对象记录

- 3.所有交易执行完后生成的Receipt列表，存储在Block中(经过压缩加密)

- 4.不同Block之间，通过前向指针ParentHash一个一个串联起来成为一个单向链表，BlockChain 结构体管理着这个链表

- 5.Block结构体基本可分为Header和Body两个部分

# 废话少说撸代码

### Block结构

```
// Block represents an entire block in the Ethereum blockchain.
type Block struct {
	// 区块头
	header       *Header
	// 叔块的区块头
	uncles       []*Header
	// 交易列表
	transactions Transactions

	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	// totalDifficulty 区块总难度  当前区块难度值 = td - lastBlock.td
	td *big.Int

	// These fields are used by package eth to track
	// inter-peer block relay.
	// 出块时间
	ReceivedAt   time.Time
	// 区块体
	ReceivedFrom interface{}
}
```
一个Block的唯一标识符就是它的hash，这里的hash是指其Header内容的RLP哈希值。在第一次计算后会缓存到hash值里。

```
// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}
...
// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *Header) Hash() common.Hash {
	return rlpHash(h)
}
..
func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
```

这里每一个Block都有一个BlockHeader，Header是Block的核心，它的成员变量全都是公共的，可以很方便的向调用者提供关于Block属性的操作。

```
// Header represents a block header in the Ethereum blockchain.
type Header struct {
	// 父区块hash，即链上上一个区块的Hash
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	// 叔块集合uncles的RLP哈希值
	UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	// 挖出区块的矿工地址
	Coinbase    common.Address `json:"miner"            gencodec:"required"`
	// MPT状态树根哈希
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	// 交易树根节点RLP哈希值
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	// 收据树根节点RLP哈希值
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	// Bloom过滤器(Filter)，用来快速判断一个参数Log对象是否存在于一组已知的Log集合中
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	// 区块难度
	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"`
	// 区块序号，相当于Bitcoin的Height
	Number      *big.Int       `json:"number"           gencodec:"required"`
	// 区块内所有Gas消耗的理论上限
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	// 区块内所有Transaction执行时所实际消耗的Gas总和
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	// 出块时间
	Time        *big.Int       `json:"timestamp"        gencodec:"required"`
	// 额外数据
	Extra       []byte         `json:"extraData"        gencodec:"required"`
	// 用于POW
	MixDigest   common.Hash    `json:"mixHash"          gencodec:"required"`
	// 用于POW 结合MixDigest生成区块哈希值
	Nonce       BlockNonce     `json:"nonce"            gencodec:"required"`
}
```
此外，以太坊将一个Block中的交易集合和叔块集合单独封装到一个Body结构中，因为他们相对于Header需要更多的内存空间,在传输和验证时为了节省时间可以和Header分开进行。

```
// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
// Body可以理解为Block里的数组成员集合，它相对于Header需要更多的内存空间,
// 所以在数据传输和验证时，往往与Header是分开进行的。
type Body struct {
	Transactions []*Transaction
	Uncles       []*Header
}
```

我们注意到,这里相比Bitcoin多了一个叔块(uncle)的概念。这里可以参考官方对[叔块](https://github.com/ethereum/wiki/wiki/Design-Rationale#uncle-incentivization)的解释。

叔块，顾名思义就是跟自己的父区块在一个高度上。我们知道相比Bitcoin，Eth将出块时间缩短到了15s左右。这样在庞大的P2P网络中就增大了同时出现同一高度区块的概率，这样就有可能使得大批矿工因为产生这样的区块而得不到奖励。因此，以太坊引入了叔块的概念，一个叔块该满足的条件为：

- 1.叔块必须是B的第K层祖先，2<= k <= 7
- 2.叔块不能是B的祖先
- 3.叔块必须拥有合法的block Header
- 4.叔块必须是不曾被包含进区块链过的

这样，以太坊的激励机制就有所改变。当一个矿工挖到一个普通区块B时，若该区块拥有叔块，该矿工将除固定区块奖励外额外再得到__固定区块奖励/32*count(uncles)__的奖励。
同时，曾经挖出叔块的矿工也将获得__(Number(uncle)+8-Number(B))*固定区块奖励/8__的叔块奖励。我们举个简单的小🌰：

A挖到一个Number为20的Block，该区块由两个叔块uncle1(B挖出Number=18)和uncle2(C挖出Number=13),此时的奖励分配为：

- A = 3 + 3/32*2 = 3.1875eth

- B = (18 + 8 - 20) * 3 / 8 = 3*6/8 = 2.25eth

- C = (13 + 8 - 20) * 3 / 8 = 3*1/8 = 0.375eth

关于区块奖励的源码今天不是重点，这里只是稍微提一下。

### Block基本操作

首先来看添加新Block的操作，代码逻辑清晰简单。

```
// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, UncleHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
func NewBlock(header *Header, txs []*Transaction, uncles []*Header, receipts []*Receipt) *Block {
	b := &Block{header: CopyHeader(header), td: new(big.Int)}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(Transactions(txs))
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.ReceiptHash = EmptyRootHash
	} else {
		b.header.ReceiptHash = DeriveSha(Receipts(receipts))
		b.header.Bloom = CreateBloom(receipts)
	}

	if len(uncles) == 0 {
		b.header.UncleHash = EmptyUncleHash
	} else {
		b.header.UncleHash = CalcUncleHash(uncles)
		b.uncles = make([]*Header, len(uncles))
		for i := range uncles {
			b.uncles[i] = CopyHeader(uncles[i])
		}
	}

	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header)}
}
```

至此，关于Block数据结构的源码就看完了。

 




