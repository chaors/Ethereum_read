
# 0x05 Transaction(交易模块) 

和Bitcoin类似，以太坊的转账流程基本是这样的：

**1.发起交易：指定目标地址和交易金额，以及必需的gas/gasLimit**

**2.交易签名：使用账户私钥对交易进行签名**

**3.提交交易：验签交易，并将交易提交到交易缓冲池**

**4.广播交易：通知以太坊虚拟机吧交易信息广播给其他节点**

### Eth Transaction结构

首先，在源码中搜索到Transaction结构的定义之处：./core/types/transaction.go

```
//交易结构体
type Transaction struct {
	//交易数据
	data txdata
	// caches
	hash atomic.Value
	size atomic.Value
	from atomic.Value
}

type txdata struct {

	//发送者发起的交易总数
	AccountNonce uint64          `json:"nonce"    gencodec:"required"`
	//交易的Gas价格
	Price        *big.Int        `json:"gasPrice" gencodec:"required"`
	//交易允许消耗的最大Gas
	GasLimit     uint64          `json:"gas"      gencodec:"required"`
	//交易接收者地址
	Recipient    *common.Address `json:"to"       rlp:"nil"` // nil means contract creation
	//交易额
	Amount       *big.Int        `json:"value"    gencodec:"required"`
	//其他数据
	Payload      []byte          `json:"input"    gencodec:"required"`

	// Signature values
	// 交易相关签名数据
	V *big.Int `json:"v" gencodec:"required"`
	R *big.Int `json:"r" gencodec:"required"`
	S *big.Int `json:"s" gencodec:"required"`

	// This is only used when marshaling to JSON.
	//交易HAsh
	Hash *common.Hash `json:"hash" rlp:"-"`
}
```

# Eth Tx转账逻辑

### 1.创建交易

首先我们曾在之前的geth基本功能一篇中使用转账命令eth.sendTransaction()进行过转账操作。
当命令行输入该指令时，geth内部实际是调用了PublicTransactionPoolAPI的sendTransaction接口:./internal/ethapi/api.go

```
// SendTransaction will create a transaction from the given arguments and
// tries to sign it with the key associated with args.To. If the given passwd isn't
// able to decrypt the key it fails.
// 发起交易
func (s *PrivateAccountAPI) SendTransaction(ctx context.Context, args SendTxArgs, passwd string) (common.Hash, error) {
	//交易参数相关判断
	if args.Nonce == nil {
		// Hold the addresse's mutex around signing to prevent concurrent assignment of
		// the same nonce to multiple accounts.
		s.nonceLock.LockAddr(args.From)
		defer s.nonceLock.UnlockAddr(args.From)
	}
	//交易签名
	signed, err := s.signTransaction(ctx, args, passwd)
	if err != nil {
		return common.Hash{}, err
	}
	//提交交易
	return submitTransaction(ctx, s.b, signed)
}
```

然后，我们看一下交易是怎么实现签名的。

```
// signTransactions sets defaults and signs the given transaction
// NOTE: the caller needs to ensure that the nonceLock is held, if applicable,
// and release it after the transaction has been submitted to the tx pool
// 交易签名
func (s *PrivateAccountAPI) signTransaction(ctx context.Context, args SendTxArgs, passwd string) (*types.Transaction, error) {
	// Look up the wallet containing the requested signer
	//获取交易发起方钱包
	account := accounts.Account{Address: args.From}
	wallet, err := s.am.Find(account)
	if err != nil {
		return nil, err
	}
	// Set some sanity defaults and terminate on failure
	if err := args.setDefaults(ctx, s.b); err != nil {
		return nil, err
	}
	// Assemble the transaction and sign with the wallet
	//组装交易
	tx := args.toTransaction()

	var chainID *big.Int
	if config := s.b.ChainConfig(); config.IsEIP155(s.b.CurrentBlock().Number()) {
		chainID = config.ChainID
	}

    //对交易进行签名
	return wallet.SignTxWithPassphrase(account, passwd, tx, chainID)
}
```

继续循着toTransaction线索去找创建交易的代码：

```
func (args *SendTxArgs) toTransaction() *types.Transaction {
	var input []byte

	//相关赋值
	if args.Data != nil {
		input = *args.Data
	} else if args.Input != nil {
		input = *args.Input
	}

	//交易接收方地址为空，创建的交易为合约交易
	if args.To == nil {
		return types.NewContractCreation(uint64(*args.Nonce), (*big.Int)(args.Value), uint64(*args.Gas), (*big.Int)(args.GasPrice), input)
	}

	//创建普通的转账交易
	return types.NewTransaction(uint64(*args.Nonce), *args.To, (*big.Int)(args.Value), uint64(*args.Gas), (*big.Int)(args.GasPrice), input)
}
```

这里终于找到了创建交易的方法NewTransaction：./core/types/transaction.go

```
//创建普通交易
func NewTransaction(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	return newTransaction(nonce, &to, amount, gasLimit, gasPrice, data)
}

//创建合约交易
func NewContractCreation(nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	return newTransaction(nonce, nil, amount, gasLimit, gasPrice, data)
}

//创建普通交易
func newTransaction(nonce uint64, to *common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	if len(data) > 0 {
		data = common.CopyBytes(data)
	}
	d := txdata{
		AccountNonce: nonce,
		Recipient:    to,
		Payload:      data,
		Amount:       new(big.Int),
		GasLimit:     gasLimit,
		Price:        new(big.Int),
		V:            new(big.Int),
		R:            new(big.Int),
		S:            new(big.Int),
	}
	if amount != nil {
		d.Amount.Set(amount)
	}
	if gasPrice != nil {
		d.Price.Set(gasPrice)
	}

	return &Transaction{data: d}
}
```

### 2.交易签名

从上面创建交易的代码细节我们已经知道对交易进行签名是通过钱包类的一个方法实现的wallet.SignTxWithPassphrase。

源码在./accounts/keystore/keystore_wallet.go

```
// SignTxWithPassphrase implements accounts.Wallet, attempting to sign the given
// transaction with the given account using passphrase as extra authentication.
// 交易签名
func (w *keystoreWallet) SignTxWithPassphrase(account accounts.Account, passphrase string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	// Make sure the requested account is contained within
	//判断账户合法性
	if account.Address != w.account.Address {
		return nil, accounts.ErrUnknownAccount
	}
	if account.URL != (accounts.URL{}) && account.URL != w.account.URL {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign
	//真正的签名
	return w.keystore.SignTxWithPassphrase(account, passphrase, tx, chainID)
}
```

继续深入到签名函数里。

```
// SignTxWithPassphrase signs the transaction if the private key matching the
// given address can be decrypted with the given passphrase.
func (ks *KeyStore) SignTxWithPassphrase(a accounts.Account, passphrase string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	// 判断账户是否解锁并获取私钥
	_, key, err := ks.getDecryptedKey(a, passphrase)
	if err != nil {
		return nil, err
	}
	defer zeroKey(key.PrivateKey)

	// Depending on the presence of the chain ID, sign with EIP155 or homestead
	// EIP155规范需要chainID参数，即平时命令行使用的“--networkid”参数
	if chainID != nil {
		return types.SignTx(tx, types.NewEIP155Signer(chainID), key.PrivateKey)
	}
	return types.SignTx(tx, types.HomesteadSigner{}, key.PrivateKey)
}
```

终于见到交易的签名函数本尊了。

```
// SignTx signs the transaction using the given signer and private key
func SignTx(tx *Transaction, s Signer, prv *ecdsa.PrivateKey) (*Transaction, error) {
	//1.对交易进行哈希
	h := s.Hash(tx)
	//2.生成签名
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}
	//3.将签名数据填充到Tx信息中
	return tx.WithSignature(s, sig)
}
```

找到这里后，就可以继续深入crypto.Sign方法看下签名是怎么根据交易哈希和私钥生成的。

```
// Sign calculates an ECDSA signature.
//
// This function is susceptible to chosen plaintext attacks that can leak
// information about the private key that is used for signing. Callers must
// be aware that the given hash cannot be chosen by an adversery. Common
// solution is to hash any input before calculating the signature.
//
// The produced signature is in the [R || S || V] format where V is 0 or 1.
//根据ECDSA算法生成签名，以字节数组的形式返回  按[R || S || V]格式
func Sign(hash []byte, prv *ecdsa.PrivateKey) (sig []byte, err error) {
	//哈希值判断
	if len(hash) != 32 {
		return nil, fmt.Errorf("hash is required to be exactly 32 bytes (%d)", len(hash))
	}
	seckey := math.PaddedBigBytes(prv.D, prv.Params().BitSize/8)
	defer zeroBytes(seckey)
	return secp256k1.Sign(hash, seckey)
}
```

生成签名后将签名填充到交易信息的R，S，V字段。

```
// WithSignature returns a new transaction with the given signature.
// This signature needs to be formatted as described in the yellow paper (v+27).
// 生成签名后将签名填充到交易信息的R，S，V字段。
func (tx *Transaction) WithSignature(signer Signer, sig []byte) (*Transaction, error) {
	//获取签名信息
	r, s, v, err := signer.SignatureValues(tx, sig)
	if err != nil {
		return nil, err
	}
	//将原有交易信息进行一份拷贝
	cpy := &Transaction{data: tx.data}
	//签名赋值
	cpy.data.R, cpy.data.S, cpy.data.V = r, s, v
	return cpy, nil
}
```

### 3.交易提交

交易签名后就可以提交到交易缓冲池，这里是通过submitTransaction()函数实现的。这里涉及到一个新的数据结构交易缓冲池TxPool，所以先来看下TxPool的结构：./core/tx_pool.go

```
// TxPool contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
//
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
type TxPool struct {
    //交易缓冲池配置
	config       TxPoolConfig
	chainconfig  *params.ChainConfig
	chain        blockChain
	gasPrice     *big.Int
	txFeed       event.Feed
	scope        event.SubscriptionScope
	chainHeadCh  chan ChainHeadEvent
	chainHeadSub event.Subscription
	signer       types.Signer
	mu           sync.RWMutex

	currentState  *state.StateDB      // Current state in the blockchain head
	pendingState  *state.ManagedState // Pending state tracking virtual nonces
	currentMaxGas uint64              // Current gas limit for transaction caps

	locals  *accountSet // Set of local transaction to exempt from eviction rules
	journal *txJournal  // Journal of local transaction to back up to disk

	//当前所有可被处理的交易列表
	pending map[common.Address]*txList   // All currently processable transactions
	//当前所有不可被处理的交易队列
	queue   map[common.Address]*txList   // Queued but non-processable transactions
	beats   map[common.Address]time.Time // Last heartbeat from each known account
	//所有的交易列表 key为交易hash
	all     *txLookup                    // All transactions to allow lookups
	//将all中的交易按照gas price进行排列的数组，gas price相同按noce升序排列
	priced  *txPricedList                // All transactions sorted by price

	wg sync.WaitGroup // for shutdown sync

	homestead bool
}
```

这里涉及到两个重要的属性pending和queue，它们的类型都是txList，所以就继续看下txList的结构。

```
// txList is a "list" of transactions belonging to an account, sorted by account
// nonce. The same type can be used both for storing contiguous transactions for
// the executable/pending queue; and for storing gapped transactions for the non-
// executable/future queue, with minor behavioral changes.
type txList struct {
	//交易的nonce值是否连续
	strict bool         // Whether nonces are strictly continuous or not
	//已排序的交易Map
	txs    *txSortedMap // Heap indexed sorted hash map of the transactions
	//最高成本交易价格
	costcap *big.Int // Price of the highest costing transaction (reset only if exceeds balance)
	//最高花费的gas限制
	gascap  uint64   // Gas limit of the highest spending transaction (reset only if exceeds block limit)
}
...
// txSortedMap is a nonce->transaction hash map with a heap based index to allow
// iterating over the contents in a nonce-incrementing way.
type txSortedMap struct {
	//包含所有交易的字典，key是交易对应nonce
	items map[uint64]*types.Transaction // Hash map storing the transaction data
	//降序排列的Nonce值数组
	index *nonceHeap                    // Heap of nonces of all the stored transactions (non-strict mode)
	//已经排序的交易缓存
	cache types.Transactions            // Cache of the transactions already sorted
}
```

交易缓冲池这里的逻辑大概是这样的：交易提交后首先是进入到txPool的queue队列缓存，然后再选择一部分交易进入peending列表进行处理。当txPool满了的时候，会根据priced的排序规则去除gas price廉价的交易来保证txPool正常运行。

我们可以看一下Eth默认的交易缓冲池配置：

```
// TxPoolConfig are the configuration parameters of the transaction pool.
type TxPoolConfig struct {
	NoLocals  bool          // Whether local transaction handling should be disabled
	Journal   string        // Journal of local transactions to survive node restarts
	Rejournal time.Duration // Time interval to regenerate the local transaction journal

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Minimum number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

//  contains the default configurations for the transaction
// pool.
// TxPool默认配置
var DefaultTxPoolConfig = TxPoolConfig{
	Journal:   "transactions.rlp",
	Rejournal: time.Hour,

	//允许进入交易池的最低gas price
	PriceLimit: 1,
	//相同Nonce交易 gas price差值超过该值，则使用新的交易
	PriceBump:  10,

	//pending列表中每个账户存储的交易处阈值，超过该数可能被认为垃圾交易
	AccountSlots: 16,
	//pending列表最大长度
	GlobalSlots:  4096,
	//queue队列中每个账户存储的交易处阈值，超过该数可能被认为垃圾交易
	AccountQueue: 64,
	//queue队列最大长度
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}
```

现在了解了txPool结构之后，我们终于可以进入正题来看submitTransaction()函数的实现了:./internal/ethapi/api.go

```
// submitTransaction is a helper function that submits tx to txPool and logs a message.
// 提交交易到交易池
func submitTransaction(ctx context.Context, b Backend, tx *types.Transaction) (common.Hash, error) {
	
	//b Backend是在eth Service初始化时创建的，在ethapiBackend(./eth/api_backend.go)
	// 通过Backend类真正实现提交交易
	if err := b.SendTx(ctx, tx); err != nil {
		return common.Hash{}, err
	}
	if tx.To() == nil {
		signer := types.MakeSigner(b.ChainConfig(), b.CurrentBlock().Number())
		from, err := types.Sender(signer, tx)
		if err != nil {
			return common.Hash{}, err
		}
		addr := crypto.CreateAddress(from, tx.Nonce())
		log.Info("Submitted contract creation", "fullhash", tx.Hash().Hex(), "contract", addr.Hex())
	} else {
		log.Info("Submitted transaction", "fullhash", tx.Hash().Hex(), "recipient", tx.To())
	}
	return tx.Hash(), nil
}
```

按图索骥，深入到Bakend.sendTx函数：

```
func (b *EthAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.eth.txPool.AddLocal(signedTx)
}
```

然后继续找到txPool的addLocal函数：

```
// AddLocal enqueues a single transaction into the pool if it is valid, marking
// the sender as a local one in the mean time, ensuring it goes around the local
// pricing constraints.
func (pool *TxPool) AddLocal(tx *types.Transaction) error {
	return pool.addTx(tx, !pool.config.NoLocals)
}
...
// addTx enqueues a single transaction into the pool if it is valid.
// 将一笔普通交易添加到TxPool中
func (pool *TxPool) addTx(tx *types.Transaction, local bool) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Try to inject the transaction and update any state
	// 将交易加入交易池queue队列
	replace, err := pool.add(tx, local)
	if err != nil {
		return err
	}
	// If we added a new transaction, run promotion checks and return
	// 通过promoteExecutables将queue中部分交易加入到pending列表中进行处理
	if !replace {
		from, _ := types.Sender(pool.signer, tx) // already validated
		pool.promoteExecutables([]common.Address{from})
	}
	return nil
}
```

首先，先去看看将交易加入到equeu队列的方法add()：

```
// add validates a transaction and inserts it into the non-executable queue for
// later pending promotion and execution. If the transaction is a replacement for
// an already pending or queued one, it overwrites the previous and returns this
// so outer code doesn't uselessly call promote.
//
// If a newly added transaction is marked as local, its sending account will be
// whitelisted, preventing any associated transaction from being dropped out of
// the pool due to pricing constraints.
func (pool *TxPool) add(tx *types.Transaction, local bool) (bool, error) {
	// If the transaction is already known, discard it
	//获取交易hash并以此判断交易池中是否已存在该笔交易
	hash := tx.Hash()
	if pool.all.Get(hash) != nil {
		log.Trace("Discarding already known transaction", "hash", hash)
		return false, fmt.Errorf("known transaction: %x", hash)
	}
	// If the transaction fails basic validation, discard it
	// 验证交易合法性
	if err := pool.validateTx(tx, local); err != nil {
		log.Trace("Discarding invalid transaction", "hash", hash, "err", err)
		invalidTxCounter.Inc(1)
		return false, err
	}
	// If the transaction pool is full, discard underpriced transactions
	// 如果交易池已满，按priced数组中gas price较低的交易剔除
	if uint64(pool.all.Count()) >= pool.config.GlobalSlots+pool.config.GlobalQueue {
		// If the new transaction is underpriced, don't accept it
		if !local && pool.priced.Underpriced(tx, pool.locals) {
			log.Trace("Discarding underpriced transaction", "hash", hash, "price", tx.GasPrice())
			underpricedTxCounter.Inc(1)
			return false, ErrUnderpriced
		}
		// New transaction is better than our worse ones, make room for it
		drop := pool.priced.Discard(pool.all.Count()-int(pool.config.GlobalSlots+pool.config.GlobalQueue-1), pool.locals)
		for _, tx := range drop {
			log.Trace("Discarding freshly underpriced transaction", "hash", tx.Hash(), "price", tx.GasPrice())
			underpricedTxCounter.Inc(1)
			pool.removeTx(tx.Hash(), false)
		}
	}
	// If the transaction is replacing an already pending one, do directly
	// 如果交易已经存在于pending列表，比较新旧交易gasPrice的差值是否超过PriceBump
	// 若超过则使用新交易代替旧交易
	from, _ := types.Sender(pool.signer, tx) // already validated
	if list := pool.pending[from]; list != nil && list.Overlaps(tx) {
		// Nonce already pending, check if required price bump is met
		inserted, old := list.Add(tx, pool.config.PriceBump)
		if !inserted {
			pendingDiscardCounter.Inc(1)
			return false, ErrReplaceUnderpriced
		}
		// New transaction is better, replace old one
		if old != nil {
			pool.all.Remove(old.Hash())
			pool.priced.Removed()
			pendingReplaceCounter.Inc(1)
		}
		pool.all.Add(tx)
		pool.priced.Put(tx)
		pool.journalTx(from, tx)

		log.Trace("Pooled new executable transaction", "hash", hash, "from", from, "to", tx.To())

		// We've directly injected a replacement transaction, notify subsystems
		go pool.txFeed.Send(NewTxsEvent{types.Transactions{tx}})

		return old != nil, nil
	}
	// New transaction isn't replacing a pending one, push into queue
	// 将交易添加到equeu队列
	replace, err := pool.enqueueTx(hash, tx)
	if err != nil {
		return false, err
	}
	// Mark local addresses and journal local transactions
	// 判断是否本地交易，保证本地交易优先被加入到TxPool
	if local {
		pool.locals.add(from)
	}
	pool.journalTx(from, tx)

	log.Trace("Pooled new future transaction", "hash", hash, "from", from, "to", tx.To())
	return replace, nil
}
```

这里对交易合法性的验证必须满足8个条件：

```
// validateTx checks whether a transaction is valid according to the consensus
// rules and adheres to some heuristic limits of the local node (price and size).
// 交易合法性验证
func (pool *TxPool) validateTx(tx *types.Transaction, local bool) error {
	// Heuristic limit, reject transactions over 32KB to prevent DOS attacks
	// 1.交易数据量必须 < 32KB
	if tx.Size() > 32*1024 {
		return ErrOversizedData
	}
	// Transactions can't be negative. This may never happen using RLP decoded
	// transactions but may occur if you create a transaction using the RPC.
	// 2.交易金额必须非负值
	if tx.Value().Sign() < 0 {
		return ErrNegativeValue
	}
	// Ensure the transaction doesn't exceed the current block limit gas.
	// 3.交易的gasLimit必须 < 交易池当前规定最大gas
	if pool.currentMaxGas < tx.Gas() {
		return ErrGasLimit
	}
	// Make sure the transaction is signed properly
	// 4.交易签名必须有效
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return ErrInvalidSender
	}
	// Drop non-local transactions under our own minimal accepted gas price
	// 5.交易的gas price必须大于交易池设置的gas price
	local = local || pool.locals.contains(from) // account may be local even if the transaction arrived from the network
	if !local && pool.gasPrice.Cmp(tx.GasPrice()) > 0 {
		return ErrUnderpriced
	}
	// Ensure the transaction adheres to nonce ordering
	// 6.交易的Nonce值必须大于链上该账户的Nonce
	if pool.currentState.GetNonce(from) > tx.Nonce() {
		return ErrNonceTooLow
	}
	// Transactor should have enough funds to cover the costs
	// cost == V + GP * GL
	// 7.交易账户余额必须 > 交易额 + gasPrice * gasLimit
	if pool.currentState.GetBalance(from).Cmp(tx.Cost()) < 0 {
		return ErrInsufficientFunds
	}
	// 8.交易的gasLimit必须 > 对应数据量所需要的最低gas水平
	intrGas, err := IntrinsicGas(tx.Data(), tx.To() == nil, pool.homestead)
	if err != nil {
		return err
	}
	if tx.Gas() < intrGas {
		return ErrIntrinsicGas
	}
	return nil
}
```

接下来继续看，交易从queue队列到pending列表又是怎么一个过程：

```
// promoteExecutables moves transactions that have become processable from the
// future queue to the set of pending transactions. During this process, all
// invalidated transactions (low nonce, low balance) are deleted.
func (pool *TxPool) promoteExecutables(accounts []common.Address) {
	// Track the promoted transactions to broadcast them at once
	var promoted []*types.Transaction

	// Gather all the accounts potentially needing updates
	if accounts == nil {
		accounts = make([]common.Address, 0, len(pool.queue))
		for addr := range pool.queue {
			accounts = append(accounts, addr)
		}
	}
	// Iterate over all accounts and promote any executable transactions
	for _, addr := range accounts {
		list := pool.queue[addr]
		if list == nil {
			continue // Just in case someone calls with a non existing account
		}
		// Drop all transactions that are deemed too old (low nonce)
		// 1.1丢弃交易nonce值 < 账户当前nonce的交易
		for _, tx := range list.Forward(pool.currentState.GetNonce(addr)) {
			hash := tx.Hash()
			log.Trace("Removed old queued transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
		}
		// Drop all transactions that are too costly (low balance or out of gas)
		// 1.2.丢弃账户余额不足的
		drops, _ := list.Filter(pool.currentState.GetBalance(addr), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			log.Trace("Removed unpayable queued transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
			queuedNofundsCounter.Inc(1)
		}
		// Gather all executable transactions and promote them
		// 3.将交易添加到pending列表
		for _, tx := range list.Ready(pool.pendingState.GetNonce(addr)) {
			hash := tx.Hash()
			if pool.promoteTx(addr, hash, tx) {
				log.Trace("Promoting queued transaction", "hash", hash)
				promoted = append(promoted, tx)
			}
		}
		// Drop all transactions over the allowed limit
		if !pool.locals.contains(addr) {
			for _, tx := range list.Cap(int(pool.config.AccountQueue)) {
				hash := tx.Hash()
				pool.all.Remove(hash)
				pool.priced.Removed()
				queuedRateLimitCounter.Inc(1)
				log.Trace("Removed cap-exceeding queued transaction", "hash", hash)
			}
		}
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			delete(pool.queue, addr)
		}
	}
	// Notify subsystem for new promoted transactions.
	if len(promoted) > 0 {
		go pool.txFeed.Send(NewTxsEvent{promoted})
	}
	// If the pending limit is overflown, start equalizing allowances
	pending := uint64(0)
	for _, list := range pool.pending {
		pending += uint64(list.Len())
	}
	//2 pending列表达到最大限量
	if pending > pool.config.GlobalSlots {
		pendingBeforeCap := pending
		// Assemble a spam order to penalize large transactors first
		spammers := prque.New()
		for addr, list := range pool.pending {
			// Only evict transactions from high rollers
			// 统计高额交易
			if !pool.locals.contains(addr) && uint64(list.Len()) > pool.config.AccountSlots {
				spammers.Push(addr, float32(list.Len()))
			}
		}
		// Gradually drop transactions from offenders
		// 逐渐驱逐高额交易
		offenders := []common.Address{}
		for pending > pool.config.GlobalSlots && !spammers.Empty() {
			// Retrieve the next offender if not local address
			offender, _ := spammers.Pop()
			offenders = append(offenders, offender.(common.Address))

			// Equalize balances until all the same or below threshold
			// 均衡各账户存储的交易数直到交易数相同

			/* 均衡交易数时采取的策略是：
				2.1.在超出交易数的账户里以交易数最少的为标准，将其他账户的交易数削减至该标准
					eg:10个账户交易数超过了AccountSlots(16),其中交易数最少的为18，则将其他9个账户的交易数削减至18
				2.2.经过1后，pengding长度依旧超过GlobalSlots，此时按照AccountSlots标准将超标的账户里交易数削减至AccountSlots
					  eg：将2.1里的10个账户的交易数都削减至AccountSlots(16)
			**/
			// 2.1
			if len(offenders) > 1 {
				// Calculate the equalization threshold for all current offenders
				// 超标账户的最低交易数
				threshold := pool.pending[offender.(common.Address)].Len()

				// Iteratively reduce all offenders until below limit or threshold reached
				// 将其他账户的交易数削减至threshold
				for pending > pool.config.GlobalSlots && pool.pending[offenders[len(offenders)-2]].Len() > threshold {
					for i := 0; i < len(offenders)-1; i++ {
						list := pool.pending[offenders[i]]
						for _, tx := range list.Cap(list.Len() - 1) {
							// Drop the transaction from the global pools too
							hash := tx.Hash()
							pool.all.Remove(hash)
							pool.priced.Removed()

							// Update the account nonce to the dropped transaction
							if nonce := tx.Nonce(); pool.pendingState.GetNonce(offenders[i]) > nonce {
								pool.pendingState.SetNonce(offenders[i], nonce)
							}
							log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
						}
						pending--
					}
				}
			}
		}
		// If still above threshold, reduce to limit or min allowance
		// 2.2 经过1的交易数均衡后，pengding长度依旧超过GlobalSlots 此时按照AccountSlots标准将超标的账户里交易数削减至AccountSlots
		if pending > pool.config.GlobalSlots && len(offenders) > 0 {
			for pending > pool.config.GlobalSlots && uint64(pool.pending[offenders[len(offenders)-1]].Len()) > pool.config.AccountSlots {
				for _, addr := range offenders {
					list := pool.pending[addr]
					for _, tx := range list.Cap(list.Len() - 1) {
						// Drop the transaction from the global pools too
						hash := tx.Hash()
						pool.all.Remove(hash)
						pool.priced.Removed()

						// Update the account nonce to the dropped transaction
						if nonce := tx.Nonce(); pool.pendingState.GetNonce(addr) > nonce {
							pool.pendingState.SetNonce(addr, nonce)
						}
						log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
					}
					pending--
				}
			}
		}
		pendingRateLimitCounter.Inc(int64(pendingBeforeCap - pending))
	}
	// If we've queued more transactions than the hard limit, drop oldest ones
	queued := uint64(0)
	for _, list := range pool.queue {
		queued += uint64(list.Len())
	}

	// 3.eqeue队列长度大于queue队列最大长度
	if queued > pool.config.GlobalQueue {
		// Sort all accounts with queued transactions by heartbeat
		// 对队列里的所有账户按最近一次心跳时间排序
		addresses := make(addresssByHeartbeat, 0, len(pool.queue))
		for addr := range pool.queue {
			if !pool.locals.contains(addr) { // don't drop locals
				addresses = append(addresses, addressByHeartbeat{addr, pool.beats[addr]})
			}
		}
		sort.Sort(addresses)

		// Drop transactions until the total is below the limit or only locals remain
		// 按顺序删除相关账户的交易，直到queue队列长度符合条件
		for drop := queued - pool.config.GlobalQueue; drop > 0 && len(addresses) > 0; {
			addr := addresses[len(addresses)-1]
			list := pool.queue[addr.address]

			addresses = addresses[:len(addresses)-1]

			// Drop all transactions if they are less than the overflow
			if size := uint64(list.Len()); size <= drop {
				for _, tx := range list.Flatten() {
					pool.removeTx(tx.Hash(), true)
				}
				drop -= size
				queuedRateLimitCounter.Inc(int64(size))
				continue
			}
			// Otherwise drop only last few transactions
			txs := list.Flatten()
			for i := len(txs) - 1; i >= 0 && drop > 0; i-- {
				pool.removeTx(txs[i].Hash(), true)
				drop--
				queuedRateLimitCounter.Inc(1)
			}
		}
	}
}
```

在这里promoteExecutables主要有三个作用：

> 1.将queue中选出符合条件的交易加入到pending中。在这之前需要对交易进行一些判断：
>> 1.1丢弃交易nonce值 < 账户当前nonce的交易
>>1.2.丢弃账户余额不足的

> 2.对pending列表进行清理，以使其满足相关配置条件。
>> 2.1在超出交易数的账户里以交易数最少的为标准，将其他账户的交易数削减至该标准 eg:10个账户交易数超过了AccountSlots(16),其中交易数最少的为18，则将其他9个账户的交易数削减至18
>>2.2经过1后，pengding长度依旧超过GlobalSlots，此时按照AccountSlots标准将超标的账户里交易数削减至AccountSlots  eg：将2.1里的10个账户的交易数都削减至AccountSlots(16)

> 3.对queue队列进行清理，以使其满足相关配置条件。
>> eqeue队列长度大于queue队列最大长度,按顺序删除相关账户的交易，直到queue队列长度符合条件

### 执行和广播交易

接着pool.txFeed.Send发送一个TxPreEvent事件，外部呢会通过SubscribeNewTxsEvent()函数来订阅该事件：

```
// SubscribeNewTxsEvent registers a subscription of NewTxsEvent and
// starts sending event to the given channel.
func (pool *TxPool) SubscribeNewTxsEvent(ch chan<- NewTxsEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}
```

在源码中全局搜索这个函数，在./miner/worker.go中发现一次SubscribeNewTxsEvent的订阅。

![SubscribeNewTxsEvent订阅](https://upload-images.jianshu.io/upload_images/830585-f1c2f149cb2c34c1.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

我们发现这里订阅了TxPreEvent事件后，开启了一个goroutine来处理该事件，进一步分析update函数，可以看到，如果当前节点不挖矿会调用commitTransactions函数提交交易；否则会调用commitNewWork函数，但其内部依然会调用commitTransactions函数提交交易。

```
func (self *worker) update() {
	defer self.txsSub.Unsubscribe()
	defer self.chainHeadSub.Unsubscribe()
	defer self.chainSideSub.Unsubscribe()

	for {
		// A real event arrived, process interesting content
		select {
		// Handle ChainHeadEvent
		case <-self.chainHeadCh:
			self.commitNewWork()

		// Handle ChainSideEvent
		case ev := <-self.chainSideCh:
			self.uncleMu.Lock()
			self.possibleUncles[ev.Block.Hash()] = ev.Block
			self.uncleMu.Unlock()

		// Handle NewTxsEvent
		case ev := <-self.txsCh:
			// Apply transactions to the pending state if we're not mining.
			//
			// Note all transactions received may not be continuous with transactions
			// already included in the current mining block. These transactions will
			// be automatically eliminated.
			if atomic.LoadInt32(&self.mining) == 0 {
				self.currentMu.Lock()
				txs := make(map[common.Address]types.Transactions)
				for _, tx := range ev.Txs {
					acc, _ := types.Sender(self.current.signer, tx)
					txs[acc] = append(txs[acc], tx)
				}
				txset := types.NewTransactionsByPriceAndNonce(self.current.signer, txs)
				//当前节点不挖矿，提交交易
				self.current.commitTransactions(self.mux, txset, self.chain, self.coinbase)
				self.updateSnapshot()
				self.currentMu.Unlock()
			} else {
				// If we're mining, but nothing is being processed, wake on new transactions
				// 当前节点为矿工节点，commitNewWork进行挖矿
				if self.config.Clique != nil && self.config.Clique.Period == 0 {
					self.commitNewWork()
				}
			}

		// System stopped
		case <-self.txsSub.Err():
			return
		case <-self.chainHeadSub.Err():
			return
		case <-self.chainSideSub.Err():
			return
		}
	}
}
```

在上面搜索SubscribeNewTxsEvent函数时，另一个调用的地方便是./eth/handler.go。这里和上面一样也是创建了一个gorountine来处理TxPreEvent事件。

```
func (pm *ProtocolManager) Start(maxPeers int) {
	pm.maxPeers = maxPeers

	// broadcast transactions
	pm.txsCh = make(chan core.NewTxsEvent, txChanSize)
	pm.txsSub = pm.txpool.SubscribeNewTxsEvent(pm.txsCh)
	go pm.txBroadcastLoop()

	// broadcast mined blocks
	pm.minedBlockSub = pm.eventMux.Subscribe(core.NewMinedBlockEvent{})
	go pm.minedBroadcastLoop()

	// start sync handlers
	go pm.syncer()
	go pm.txsyncLoop()
}
...
func (pm *ProtocolManager) txBroadcastLoop() {
	for {
		select {
		case event := <-pm.txsCh:
			pm.BroadcastTxs(event.Txs)

		// Err() channel will be closed when unsubscribing.
		case <-pm.txsSub.Err():
			return
		}
	}
}
...
// BroadcastTxs will propagate a batch of transactions to all peers which are not known to
// already have the given transaction.
func (pm *ProtocolManager) BroadcastTxs(txs types.Transactions) {
	var txset = make(map[*peer]types.Transactions)

	// Broadcast transactions to a batch of peers not knowing about it
	for _, tx := range txs {
		peers := pm.peers.PeersWithoutTx(tx.Hash())
		for _, peer := range peers {
			txset[peer] = append(txset[peer], tx)
		}
		log.Trace("Broadcast transaction", "hash", tx.Hash(), "recipients", len(peers))
	}
	// FIXME include this again: peers = peers[:int(math.Sqrt(float64(len(peers))))]
	for peer, txs := range txset {
		peer.AsyncSendTransactions(txs)
	}
}
```

至此，一笔交易从发起到构建到签名验证以及缓存到交易池然后广播给其他节点的整个流程的逻辑就看完了。
















