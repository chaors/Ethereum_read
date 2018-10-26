# 以太坊源码研读0x08 Mining&Consensus 深度剖析 

POW的概念我们都熟悉，在公链实战系列里也实现过简单的POW算法。这里开始从源码角度分析以太坊区块的诞生历程。

# Miner结构
一个合法的Block是矿工通过POW来挖出来的，所以这里的切入点就是miner这个类。

```
// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	// 事件锁
	mux *event.TypeMux

	// 真正干活的人
	worker *worker
	// 矿工地址
	coinbase common.Address
	// 表示正在挖矿的状态
	mining   int32
	// Backend对象，Backend是一个自定义接口封装了所有挖矿所需方法
	eth      Backend
	// 共识引擎 以太坊有两种共识引擎ethash和clique
	engine   consensus.Engine

	// 是否能够开始挖矿
	canStart    int32 // can start indicates whether we can start the mining operation
	// 同步后是否应该开始挖矿
	shouldStart int32 // should start indicates whether we should start after sync
}
```
miner只是以太坊对外实现mining功能的开放类，真正干活的是worker。所以，继续深入看看worker的结构。

```
// worker is the main object which takes care of applying messages to the new state
type worker struct {
	// 链的配置属性
	config *params.ChainConfig
	// 前面提到的共识引擎
	engine consensus.Engine
	// 同步锁
	mu sync.Mutex

	// update loop
	mux          *event.TypeMux
	txsCh        chan core.NewTxsEvent
	txsSub       event.Subscription
	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription
	chainSideCh  chan core.ChainSideEvent
	chainSideSub event.Subscription
	wg           sync.WaitGroup

	// Agent的map集合
	agents map[Agent]struct{}
	recv   chan *Result

	eth     Backend
	chain   *core.BlockChain
	proc    core.Validator
	chainDb ethdb.Database

	coinbase common.Address
	extra    []byte

	currentMu sync.Mutex
	current   *Work

	snapshotMu    sync.RWMutex
	snapshotBlock *types.Block
	snapshotState *state.StateDB

	uncleMu        sync.Mutex
	possibleUncles map[common.Hash]*types.Block

	// 本地挖出的有待确认的区块
	unconfirmed *unconfirmedBlocks // set of locally mined blocks pending canonicalness confirmations

	// atomic status counters
	mining int32
	atWork int32
}
```

这里，我们来着重看一下params.ChainConfig这个结构，顾名思义，它是链的配置属性，其中定义了一些针对以太坊历史问题的相关配置。

```

// ChainConfig is stored in the database on a per block basis. This means
// that any network, identified by its genesis block, can have its own
// set of configuration options.
type ChainConfig struct {
	// 标识当前链，主键唯一id 也用来防止replay attack重放攻击
	ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection
	// 以太坊发展蓝图中的一个阶段,当前阶段为Homestead
	// 第一阶段为以太坊面世代号frontier，第二阶段为Homestead即当前阶段
	// 第三阶段为Metropolis(大都会)，Metropolis又分为Byzantium(拜占庭硬分叉，引入新型零知识证明算法和pos共识)，
	// 然后是constantinople(君士坦丁堡硬分叉，eth正是应用pow和pos混合链)
	// 第四阶段为Serenity(宁静)，最终稳定版的以太坊
	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"` // Homestead switch block (nil = no fork, 0 = already homestead)

	// TheDao硬分叉切换，2017年6月18日应对DAO危机做出的调整
	DAOForkBlock   *big.Int `json:"daoForkBlock,omitempty"`   // TheDAO hard-fork switch block (nil = no fork)
	// 节点是否支持TheDao硬分叉
	DAOForkSupport bool     `json:"daoForkSupport,omitempty"` // Whether the nodes supports or opposes the DAO hard-fork

	// EIP150 implements the Gas price changes (https://github.com/ethereum/EIPs/issues/150)
	// eth改善方案硬分叉  没有硬分叉的置0
	EIP150Block *big.Int    `json:"eip150Block,omitempty"` // EIP150 HF block (nil = no fork)
	EIP150Hash  common.Hash `json:"eip150Hash,omitempty"`  // EIP150 HF hash (needed for header only clients as only gas pricing changed)

	EIP155Block *big.Int `json:"eip155Block,omitempty"` // EIP155 HF block
	EIP158Block *big.Int `json:"eip158Block,omitempty"` // EIP158 HF block

	ByzantiumBlock      *big.Int `json:"byzantiumBlock,omitempty"`      // Byzantium switch block (nil = no fork, 0 = already on byzantium)
	ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"` // Constantinople switch block (nil = no fork, 0 = already activated)

	// Various consensus engines
	Ethash *EthashConfig `json:"ethash,omitempty"`
	Clique *CliqueConfig `json:"clique,omitempty"`
}
```

接着在worker里有一个agent代理，他们的关系应该是这样的。一个miner有一个worker，一个worker又同时拥有多个agent。这里的单个agent可以完成单个区块的mining，worker的多个agent间应该是竞争关系。

```
// Agent can register themself with the worker
type Agent interface {
	Work() chan<- *Work
	SetReturnCh(chan<- *Result)
	Stop()
	Start()
	GetHashRate() int64
}
```

Agent接口的定义下面又有一个work结构，work主要用来表示挖掘一个区块时候所需要的数据环境。

```
// Work is the workers current environment and holds
// all of the current state information
type Work struct {
	// 链的配置属性
	config *params.ChainConfig
	signer types.Signer

	// 数据库状态
	state     *state.StateDB // apply state changes here
	// 祖先集，用来验证叔父块有效性
	ancestors *set.Set       // ancestor set (used for checking uncle parent validity)
	// 家庭集，用来验证叔块无效
	family    *set.Set       // family set (used for checking uncle invalidity)
	// 叔块集
	uncles    *set.Set       // uncle set
	// 交易量
	tcount    int            // tx count in cycle
	// 用于打包交易的可用天然气
	gasPool   *core.GasPool  // available gas used to pack transactions
	
	// 新区快
	Block *types.Block // the new block

	// 区块头
	header   *types.Header
	txs      []*types.Transaction
	receipts []*types.Receipt

	createdAt time.Time
}
```

这里有两个类实现了agent接口，分别是CpuAgent和RemoteAgent。CpuAgent是用cpu进行挖矿操作，RemoteAgent是远程挖矿，它提供了一套RPC接口来实现远程矿工进行采矿的功能。

```
type CpuAgent struct {
	// 同步锁
	mu sync.Mutex

	// work通道
	workCh        chan *Work
	// 结构体通道对象
	stop          chan struct{}
	quitCurrentOp chan struct{}
	// Result指针通道
	returnCh      chan<- *Result

	// 共识引擎
	chain  consensus.ChainReader
	engine consensus.Engine

	// 当前agent是否在挖矿
	isMining int32 // isMining indicates whether the agent is currently mining
}
...
type RemoteAgent struct {
	mu sync.Mutex

	quitCh   chan struct{}
	workCh   chan *Work
	returnCh chan<- *Result

	chain       consensus.ChainReader
	engine      consensus.Engine
	currentWork *Work
	work        map[common.Hash]*Work

	hashrateMu sync.RWMutex
	hashrate   map[common.Hash]hashrate

	running int32 // running indicates whether the agent is active. Call atomically
}
```

### 挖矿逻辑

开始挖矿首先要实例化一个miner对象。

```
// 创建miner对象
func New(eth Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine) *Miner {
	miner := &Miner{
		eth:      eth,
		mux:      mux,
		engine:   engine,
		// 创建一个worker
		worker:   newWorker(config, engine, common.Address{}, eth, mux),
		canStart: 1,
	}
	// 注册newCpuAgent对象
	miner.Register(NewCpuAgent(eth.BlockChain(), engine))
	go miner.update()

	return miner
}
```

接着为实例化的miner对象创建一个worker对象来真正地干活。

```
// 为miner创建worker
func newWorker(config *params.ChainConfig, engine consensus.Engine, coinbase common.Address, eth Backend, mux *event.TypeMux) *worker {
	worker := &worker{
		config:         config,
		engine:         engine,
		eth:            eth,
		mux:            mux,
		// NewTxsEvent面熟吧，前面讲交易时 TxPool会发出该事件，当一笔交易被放入到交易池
		// 这时候如果work空闲会把Tx放到work.txs准备下一次打包进块
		txsCh:          make(chan core.NewTxsEvent, txChanSize),
		// ChainHeadEvent事件，表示已经有一个块作为链头 work.ipdate监听到该事件会继续挖矿
		chainHeadCh:    make(chan core.ChainHeadEvent, chainHeadChanSize),
		// ChainSideEvent事件，表示一个新块作为链的旁支可能会被放入possibleUncles中
		chainSideCh:    make(chan core.ChainSideEvent, chainSideChanSize),
		// 区块链数据库
		chainDb:        eth.ChainDb(),

		recv:           make(chan *Result, resultQueueSize),
		chain:          eth.BlockChain(),
		proc:           eth.BlockChain().Validator(),
		// 可能的叔块
		possibleUncles: make(map[common.Hash]*types.Block),
		coinbase:       coinbase,
		agents:         make(map[Agent]struct{}),
		// 挖出的未被确认的区块
		unconfirmed:    newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
	}
	// Subscribe NewTxsEvent for tx pool
	worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
	// Subscribe events for blockchain
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.chainSideSub = eth.BlockChain().SubscribeChainSideEvent(worker.chainSideCh)
	go worker.update()

	go worker.wait()
	worker.commitNewWork()

	return worker
}
```

从上面看出，worker.update方法来处理上述几个event事件。我们上次分析交易模块时对交易的提交也是在这处理的。

那么新区块的写入是在哪操作的呢？我们来来看worker.wait便能看出点端倪。

```

func (self *worker) wait() {
	for {
		for result := range self.recv {
			atomic.AddInt32(&self.atWork, -1)

			if result == nil {
				continue
			}
			block := result.Block
			work := result.Work

			// Update the block hash in all logs since it is now available and not when the
			// receipt/log of individual transactions were created.
			// 更新所有日志的块哈希
			for _, r := range work.receipts {
				for _, l := range r.Logs {
					l.BlockHash = block.Hash()
				}
			}
			for _, log := range work.state.Logs() {
				log.BlockHash = block.Hash()
			}
			stat, err := self.chain.WriteBlockWithState(block, work.receipts, work.state)
			if err != nil {
				log.Error("Failed writing block to chain", "err", err)
				continue
			}
			// Broadcast the block and announce chain insertion event
			// 广播新区块并宣布链插入事件
			self.mux.Post(core.NewMinedBlockEvent{Block: block})
			var (
				events []interface{}
				logs   = work.state.Logs()
			)
			events = append(events, core.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})

			if stat == core.CanonStatTy {
				events = append(events, core.ChainHeadEvent{Block: block})
			}

			self.chain.PostChainEvents(events, logs)

			// Insert the block into the set of pending ones to wait for confirmations
			// 将数据插入待处理块中，等待确认
			self.unconfirmed.Insert(block.NumberU64(), block.Hash())
		}
	}
}
```

接着回到miner的创建，在创建完worker后，会为worker注册agent。

```
func NewCpuAgent(chain consensus.ChainReader, engine consensus.Engine) *CpuAgent {
	miner := &CpuAgent{
		chain:  chain,
		engine: engine,
		stop:   make(chan struct{}, 1),
		workCh: make(chan *Work, 1),
	}
	return miner
}
...
func (self *Miner) Register(agent Agent) {
	if self.Mining() {
		agent.Start()
	}
	self.worker.register(agent)
}
...
func (self *worker) register(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.agents[agent] = struct{}{}
	agent.SetReturnCh(self.recv)
}
```

接下来在miner.update方法中开始挖矿。

```
// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
// update会跟踪下载程序事件。 请注意，这是一次性更新循环。
// 一旦广播“完成”或“失败”，事件就会被取消注册并退出循环。
// 这可以防止主要的安全漏洞，外部各方可以使用块来阻止你
// 并且只要DOS继续就停止你的挖掘操作
func (self *Miner) update() {
	events := self.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
out:
	for ev := range events.Chan() {
		switch ev.Data.(type) {
		// 下载开始
		case downloader.StartEvent:
			atomic.StoreInt32(&self.canStart, 0)
			if self.Mining() {
				self.Stop()
				atomic.StoreInt32(&self.shouldStart, 1)
				log.Info("Mining aborted due to sync")
			}
			// 下载完成或失败
		case downloader.DoneEvent, downloader.FailedEvent:
			shouldStart := atomic.LoadInt32(&self.shouldStart) == 1

			atomic.StoreInt32(&self.canStart, 1)
			atomic.StoreInt32(&self.shouldStart, 0)
			if shouldStart {
				// 开始挖矿
				self.Start(self.coinbase)
			}
			// unsubscribe. we're only interested in this event once
			events.Unsubscribe()
			// stop immediately and ignore all further pending events
			break out
		}
	}
}
...
func (self *Miner) Start(coinbase common.Address) {
	atomic.StoreInt32(&self.shouldStart, 1)
	self.SetEtherbase(coinbase)

	if atomic.LoadInt32(&self.canStart) == 0 {
		log.Info("Network syncing, will start miner afterwards")
		return
	}
	atomic.StoreInt32(&self.mining, 1)

	log.Info("Starting mining operation")
	// 真正开始挖矿
	self.worker.start()
	self.worker.commitNewWork()
}
```

worker.start()函数表示，真正进行挖矿的是worker。继续深入到对应代码。

```
func (self *worker) start() {
	self.mu.Lock()
	defer self.mu.Unlock()

	atomic.StoreInt32(&self.mining, 1)

	// spin up agents
	// 遍历所有的agent，通知agent开始挖矿
	for agent := range self.agents {
		agent.Start()
	}
}
```

然后，还不是真正挖矿的代码。还得继续深入虎穴来探究agent内部的start。

```
func (self *CpuAgent) Start() {
	if !atomic.CompareAndSwapInt32(&self.isMining, 0, 1) {
		return // agent already started
	}
	go self.update()
}

func (self *CpuAgent) update() {
out:
	for {
		select {
		// 检测是否有工作信号进入
		case work := <-self.workCh:
			self.mu.Lock()
			if self.quitCurrentOp != nil {
				close(self.quitCurrentOp)
			}
			self.quitCurrentOp = make(chan struct{})
			go self.mine(work, self.quitCurrentOp)
			self.mu.Unlock()
			// 监测停止信号
		case <-self.stop:
			self.mu.Lock()
			if self.quitCurrentOp != nil {
				close(self.quitCurrentOp)
				self.quitCurrentOp = nil
			}
			self.mu.Unlock()
			break out
		}
	}
}

func (self *CpuAgent) mine(work *Work, stop <-chan struct{}) {

	// 通过共识引擎算法来挖矿
	if result, err := self.engine.Seal(self.chain, work.Block, stop); result != nil {
		log.Info("Successfully sealed new block", "number", result.Number(), "hash", result.Hash())
		self.returnCh <- &Result{work, result}
	} else {
		if err != nil {
			log.Warn("Block sealing failed", "err", err)
		}
		self.returnCh <- nil
	}
}
```
这里，agent.update和miner.update逻辑类似。好在这里终于看到了最终挖矿时通过agent封装的共识引擎来实现的。

其次，还有一个worker. commitNewWork()方法，它主要完成待挖掘区块的数据组装。

```
// 完成待挖掘区块的数据组装
func (self *worker) commitNewWork() {

	// 相关锁设置
	self.mu.Lock()
	defer self.mu.Unlock()
	self.uncleMu.Lock()
	defer self.uncleMu.Unlock()
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	tstart := time.Now()
	parent := self.chain.CurrentBlock()

	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		tstamp = parent.Time().Int64() + 1
	}
	// this will ensure we're not going off too far in the future
	// 确保新区块的产生不会花费太多时间
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	// 组装区块头信息
	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.CalcGasLimit(parent),
		Extra:      self.extra,
		Time:       big.NewInt(tstamp),
	}
	// Only set the coinbase if we are mining (avoid spurious block rewards)
	// 如果正在挖掘，设置coinbase
	if atomic.LoadInt32(&self.mining) == 1 {
		header.Coinbase = self.coinbase
	}
	// 确保区块头信息准备好
	if err := self.engine.Prepare(self.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return
	}
	// If we are care about TheDAO hard-fork check whether to override the extra-data or not
	// TheDAO硬分叉相关设置
	if daoBlock := self.config.DAOForkBlock; daoBlock != nil {
		// Check whether the block is among the fork extra-override range
		limit := new(big.Int).Add(daoBlock, params.DAOForkExtraRange)
		// 根据新区块Number判断是否受TheDAO硬分叉的影响
		if header.Number.Cmp(daoBlock) >= 0 && header.Number.Cmp(limit) < 0 {
			// Depending whether we support or oppose the fork, override differently
			if self.config.DAOForkSupport {
				// 支持TheDAO硬分叉
				header.Extra = common.CopyBytes(params.DAOForkBlockExtra)
			} else if bytes.Equal(header.Extra, params.DAOForkBlockExtra) {
				header.Extra = []byte{} // If miner opposes, don't let it use the reserved extra-data
			}
		}
	}
	// Could potentially happen if starting to mine in an odd state.
	err := self.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	// Create the current work task and check any fork transitions needed
	// 创建当前所需的工作环境work
	work := self.current
	// 给work设置相关硬分叉设置
	if self.config.DAOForkSupport && self.config.DAOForkBlock != nil && self.config.DAOForkBlock.Cmp(header.Number) == 0 {
		misc.ApplyDAOHardFork(work.state)
	}
	// 准备新区块的交易列表
	pending, err := self.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return
	}
	// 交易按价格和nonce值排序
	txs := types.NewTransactionsByPriceAndNonce(self.current.signer, pending)
	// 提交交易
	work.commitTransactions(self.mux, txs, self.chain, self.coinbase)

	// compute uncles for the new block.
	// 为新的区块计算叔块
	var (
		uncles    []*types.Header
		badUncles []common.Hash
	)
	for hash, uncle := range self.possibleUncles {
		if len(uncles) == 2 {
			break
		}
		if err := self.commitUncle(work, uncle.Header()); err != nil {
			log.Trace("Bad uncle found and will be removed", "hash", hash)
			log.Trace(fmt.Sprint(uncle))

			badUncles = append(badUncles, hash)
		} else {
			log.Debug("Committing new uncle to block", "hash", hash)
			uncles = append(uncles, uncle.Header())
		}
	}
	for _, hash := range badUncles {
		delete(self.possibleUncles, hash)
	}
	// Create the new block to seal with the consensus engine
	// 使用共识引擎对新区块进行赋值，包括Header.Root, TxHash, ReceiptHash, UncleHash几个属性
	if work.Block, err = self.engine.Finalize(self.chain, header, work.state, work.txs, uncles, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return
	}
	// We only care about logging if we're actually mining.
	if atomic.LoadInt32(&self.mining) == 1 {
		log.Info("Commit new mining work", "number", work.Block.Number(), "txs", work.tcount, "uncles", len(uncles), "elapsed", common.PrettyDuration(time.Since(tstart)))
		self.unconfirmed.Shift(work.Block.NumberU64() - 1)
	}
	// 加载工作环境
	self.push(work)
	self.updateSnapshot()
}
```

总结下思路，首先挖矿这里又三个角色：miner，worker和agent。miner是以太坊封装的对外的挖矿接口，worker是给miner干活的，agent是真正实现挖矿的东东。一个miner都会有一个worker属性，一个worker又有多个agent来通过共识引擎实现真正挖矿，这里还有个work来存储挖矿所需要的数据环境。

当要开始挖矿时，miner会创建一个worker，并为之注册对应的agent。然后worker将一个work对象发送给agent，所有的agent根据共识引擎来挖矿，当一个agent完成mine时会将一个授权的block加上对应的work组成一个result对象返回给worker。

# 以太坊共识引擎

上面一堆讲的是以太坊的挖矿逻辑，还没真正涉及到POW的实现。上面也讲到过以太坊的共识引擎有两种，ethash和clique。ethash本质上就是一种POW算法，clique则是POA(ProofOfAuthortiy)算法。目前以太坊采用的是ethash共识引擎，也就是POW算法。

## Ethash算法

### 原理
说到这想起一个题外话，之前看到网上有人问比特币矿机能用来挖以太币吗。怎么说呢，有钱任性的话也是可以的，因为这样做的结果是入不敷出。这是为什么呢？

Bitcoin的POW是完全基于算力的，而Ethash则是基于内存和算力的，它和计算机的内存大小和内存带宽正相关。计算能力再强，它每次读取内存的带宽是有限的，这就是为什么即使用来昂贵的ASIC矿机来挖以太币，收益也不会比PC号多少。但是 ，道高一尺魔高一丈，据说比特大陆已经研发出用于以太坊的专业矿机，不过价格不菲每台800美元。题外话就说到这，接着回归正题。

我们知道POW算法依赖一个nonce值输入算法来得到一个低于困难度阈值的哈希值，Eth引入了__DAG__来存储依靠nonce和区块标题的固定资源的子集，DAG资源大约占用1GB大小的内存。在以太坊中每3000个区块会生成一个DAG，这个DAG被称为__epoch__(大约5.2d，125h)。

ethash依赖于DAG实现POW，一般会预先计算DAG来避免在每个epoch过渡时发生过长的等待时间。geth执行自动的DAG生成，每次维持两个DAG来保障epoch过渡流畅

ethash算法，又叫Dashimoto (Dagger-Hashimoto)，是Hashimoto算法结合Dagger之后产成的一个变种。其基本原理可以用一个公式表示：

> #### RAND(h, nonce) <= M / d

这里h表示区块头的哈希，nonce表示一个自增的变量，RAND表示经过一系列算法生成一个类似随机数的函数。
M表示一个极大的数，d则是当前区块的难度值header.diffculty。

ethash的大概流程是这样的：

- __1.先根据block number以及block header计算出一个种子值seed__

- 2.__使用seed产生一个32MB的伪随机数集cache__

- 3.__根据cache生成一个1GB的数据集DAG(可以根据cache快速定位DAG中指定位置的元素，所以一般轻客户端保存cache，完整客户端保存DAG)__

- 4.__从DAG中随机选择元素对其进行hash，然后判断哈希值是否小于给定值__

- 5.__cache和DAG每个周期(1000个块)更新一次。DAG从1GB开始随着时间线性增长，现在好像达到20多GB了__

### 源码撸起来

```
// Ethash is a consensus engine based on proof-of-work implementing the ethash
// algorithm.
type Ethash struct {

	// ethash配置
	config Config

	// 内存缓存，可反复使用避免再生太频繁
	caches   *lru // In memory caches to avoid regenerating too often
	// 内存数据集
	datasets *lru // In memory datasets to avoid regenerating too often

	// Mining related fields
	// 随机工具，用来生成种子
	rand     *rand.Rand    // Properly seeded random source for nonces
	// 挖矿的线程数
	threads  int           // Number of threads to mine on if mining
	// 挖矿通道
	update   chan struct{} // Notification channel to update mining parameters
	// 平均哈希率
	hashrate metrics.Meter // Meter tracking the average hashrate

	// The fields below are hooks for testing
	// 共享pow,无法再生缓存
	shared    *Ethash       // Shared PoW verifier to avoid cache regeneration
	// 未通过pow的区块号，包括fakeMode
	fakeFail  uint64        // Block number which fails PoW check even in fake mode
	// 验证工作返回消息前的延迟时间
	fakeDelay time.Duration // Time delay to sleep for before returning from verify
	
	// 同步锁
	lock sync.Mutex // Ensures thread safety for the in-memory caches and mining fields
}

// New creates a full sized ethash PoW scheme.
// 生成ethash对象
func New(config Config) *Ethash {
	if config.CachesInMem <= 0 {
		log.Warn("One ethash cache must always be in memory", "requested", config.CachesInMem)
		config.CachesInMem = 1
	}
	if config.CacheDir != "" && config.CachesOnDisk > 0 {
		log.Info("Disk storage enabled for ethash caches", "dir", config.CacheDir, "count", config.CachesOnDisk)
	}
	if config.DatasetDir != "" && config.DatasetsOnDisk > 0 {
		log.Info("Disk storage enabled for ethash DAGs", "dir", config.DatasetDir, "count", config.DatasetsOnDisk)
	}
	return &Ethash{
		config:   config,
		caches:   newlru("cache", config.CachesInMem, newCache),
		datasets: newlru("dataset", config.DatasetsInMem, newDataset),
		update:   make(chan struct{}),
		hashrate: metrics.NewMeter(),
	}
}
```

这里涉及到两个存储数据的类cache和dataset,他们的数据结构类似。我猜想这里的cache对应ethash需要的缓存cache，dataset对应相应的DAG。

```
// cache wraps an ethash cache with some metadata to allow easier concurrent use.
// cache使用一些元数据包装ethash缓存，以便更容易并发使用。
type cache struct {
	// 属于哪一个epoch
	epoch uint64    // Epoch for which this cache is relevant
	// 该内存存储于磁盘的文件对象
	dump  *os.File  // File descriptor of the memory mapped cache
	// 内存映射
	mmap  mmap.MMap // Memory map itself to unmap before releasing
	// 实际使用的内存
	cache []uint32  // The actual cache data content (may be memory mapped)
	once  sync.Once // Ensures the cache is generated only once
}
...
// dataset wraps an ethash dataset with some metadata to allow easier concurrent use.
type dataset struct {
	epoch   uint64    // Epoch for which this cache is relevant
	dump    *os.File  // File descriptor of the memory mapped cache
	mmap    mmap.MMap // Memory map itself to unmap before releasing
	dataset []uint32  // The actual cache data content
	once    sync.Once // Ensures the cache is generated only once
}
```

我们上面分析挖矿逻辑时，分析到了agent内部是通过Engine.seal来实现挖矿的。这里我们就可以来看看ethash的Seal()实现。

```
// Seal implements consensus.Engine, attempting to find a nonce that satisfies
// the block's difficulty requirements.
// Seal实现了共识引擎，找到满足块的难度要求的Nonce值。
func (ethash *Ethash) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	// If we're running a fake PoW, simply return a 0 nonce immediately
	// ModeFake模式立即返回
	if ethash.config.PowMode == ModeFake || ethash.config.PowMode == ModeFullFake {
		header := block.Header()
		header.Nonce, header.MixDigest = types.BlockNonce{}, common.Hash{}
		return block.WithSeal(header), nil
	}
	// If we're running a shared PoW, delegate sealing to it
	// 共享模式，转到它的共享对象执行Seal操作
	if ethash.shared != nil {
		return ethash.shared.Seal(chain, block, stop)
	}
	// Create a runner and the multiple search threads it directs
	// 创建runner和它指挥的多重搜索线程
	abort := make(chan struct{})
	found := make(chan *types.Block)

	// 线程锁
	ethash.lock.Lock()
	// 获取挖矿线程
	threads := ethash.threads
	if ethash.rand == nil {
		// 获取种子seed
		seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			ethash.lock.Unlock()
			return nil, err
		}
		// 执行成功，拿到合法种子seed，通过其获得rand对象，赋值
		ethash.rand = rand.New(rand.NewSource(seed.Int64()))
	}
	ethash.lock.Unlock()
	if threads == 0 {
		// 如果设定的线程数为0，则实际线程数同CPU数
		threads = runtime.NumCPU()
	}
	if threads < 0 {
		// 允许在本地/远程周围禁用本地挖掘而无需额外的逻辑
		threads = 0 // Allows disabling local mining without extra logic around local/remote
	}

	// 创建一个计数的信号量
	var pend sync.WaitGroup
	for i := 0; i < threads; i++ {
		//信号量赋值
		pend.Add(1)
		go func(id int, nonce uint64) {
			// 信号量值减1
			defer pend.Done()
			// 挖矿工作
			ethash.mine(block, id, nonce, abort, found)
		}(i, uint64(ethash.rand.Int63()))
	}
	// Wait until sealing is terminated or a nonce is found
	// 一直等到找到符合条件的nonce值
	var result *types.Block
	select {
	case <-stop:
		// Outside abort, stop all miner threads
		// 停止信号
		close(abort)
	case result = <-found:
		// One of the threads found a block, abort all others
		// 其中有线程找到了合法区块
		close(abort)
	case <-ethash.update:
		// Thread count was changed on user request, restart
		// 重启信号
		close(abort)
		pend.Wait()
		return ethash.Seal(chain, block, stop)
	}
	// Wait for all miners to terminate and return the block
	// 等待所有矿工终止并返回该区块
	// Wait判断信号量计数器大于0，就会阻塞
	pend.Wait()
	return result, nil
}

// mine is the actual proof-of-work miner that searches for a nonce starting from
// seed that results in correct final block difficulty.
// 实际的POW，从种子开始搜索一个nonce，直到正确的合法区块出现
func (ethash *Ethash) mine(block *types.Block, id int, seed uint64, abort chan struct{}, found chan *types.Block) {
	// Extract some data from the header
	// 从区块头中取出一些数据
	var (
		header  = block.Header()
		hash    = header.HashNoNonce().Bytes()
		target  = new(big.Int).Div(maxUint256, header.Difficulty)
		number  = header.Number.Uint64()
		dataset = ethash.dataset(number)
	)
	// Start generating random nonces until we abort or find a good one
	// 开始生成随机的随机数，直到我们中止或找到一个合法的
	var (
		// 初始化一个变量来表示尝试次数
		attempts = int64(0)
		// 初始化nonce值，后面该值会递增
		nonce    = seed
	)
	logger := log.New("miner", id)
	logger.Trace("Started ethash search for new nonces", "seed", seed)
search:
	for {
		select {
		case <-abort:
			// Mining terminated, update stats and abort
			// 停止信号
			logger.Trace("Ethash nonce search aborted", "attempts", nonce-seed)
			ethash.hashrate.Mark(attempts)
			break search

		default:
			// We don't have to update hash rate on every nonce, so update after after 2^X nonces
			// 不必更新每个nonce的哈希率，所以在2 ^ X nonces之后更新
			attempts++
			if (attempts % (1 << 15)) == 0 {
				// 尝试次数达到2^15，更新hashrate 
				ethash.hashrate.Mark(attempts)
				attempts = 0
			}
			// Compute the PoW value of this nonce
			// 计算当前nonce的pow值
			digest, result := hashimotoFull(dataset.dataset, hash, nonce)
			if new(big.Int).SetBytes(result).Cmp(target) <= 0 {
				// Correct nonce found, create a new header with it
				// 找到合法的nonce值，为header赋值
				header = types.CopyHeader(header)
				header.Nonce = types.EncodeNonce(nonce)
				header.MixDigest = common.BytesToHash(digest)

				// Seal and return a block (if still needed)
				select {
				case found <- block.WithSeal(header):
					logger.Trace("Ethash nonce found and reported", "attempts", nonce-seed, "nonce", nonce)
				case <-abort:
					logger.Trace("Ethash nonce found but discarded", "attempts", nonce-seed, "nonce", nonce)
				}
				break search
			}
			nonce++
		}
	}
	// Datasets are unmapped in a finalizer. Ensure that the dataset stays live
	// during sealing so it's not unmapped while being read.
	runtime.KeepAlive(dataset)
}
```

在mine()中hashimotoFull()是为nonce值计算pow的算法，我们继续深入到这个函数中。

```
// hashimotoFull aggregates data from the full dataset (using the full in-memory
// dataset) in order to produce our final value for a particular header hash and
// nonce.
// 在传入的数据集中通过hash和nonce值计算加密值
func hashimotoFull(dataset []uint32, hash []byte, nonce uint64) ([]byte, []byte) {
	// 定义一个lookup函数，用于在数据集中查找数据
	lookup := func(index uint32) []uint32 {
		offset := index * hashWords
		return dataset[offset : offset+hashWords]
	}

	// 将原始数据集进行了读取分割，然后传给hashimoto函数
	return hashimoto(hash, nonce, uint64(len(dataset))*4, lookup)
}
...
// hashimoto aggregates data from the full dataset in order to produce our final
// value for a particular header hash and nonce.
// 在传入的数据集中通过hash和nonce值计算加密值
func hashimoto(hash []byte, nonce uint64, size uint64, lookup func(index uint32) []uint32) ([]byte, []byte) {
	// Calculate the number of theoretical rows (we use one buffer nonetheless)
	// 计算数据集理论的行数
	rows := uint32(size / mixBytes)

	// Combine header+nonce into a 64 byte seed
	// 合并header和nonce到一个40bytes的seed
	seed := make([]byte, 40)
	copy(seed, hash)
	binary.LittleEndian.PutUint64(seed[32:], nonce)

	// 将seed进行Keccak512加密
	seed = crypto.Keccak512(seed)
	// 从seed中获取区块头
	seedHead := binary.LittleEndian.Uint32(seed)

	// Start the mix with replicated seed
	// 开始与重复seed的混合 mixBytes/4 = 128/4=32
	// 长度32，元素uint32 mix占4*32=128bytes
	mix := make([]uint32, mixBytes/4)
	for i := 0; i < len(mix); i++ {
		mix[i] = binary.LittleEndian.Uint32(seed[i%16*4:])
	}
	// Mix in random dataset nodes
	// 定义一个temp，与mix结构相同，长度相同
	temp := make([]uint32, len(mix))

	for i := 0; i < loopAccesses; i++ {
		parent := fnv(uint32(i)^seedHead, mix[i%len(mix)]) % rows
		for j := uint32(0); j < mixBytes/hashBytes; j++ {
			copy(temp[j*hashWords:], lookup(2*parent+j))
		}
		// 将mix中所有元素都与temp中对应位置的元素进行FNV hash运算
		fnvHash(mix, temp)
	}
	// Compress mix
	// 对Mix进行混淆
	for i := 0; i < len(mix); i += 4 {
		mix[i/4] = fnv(fnv(fnv(mix[i], mix[i+1]), mix[i+2]), mix[i+3])
	}
	// 保留8个字节有效数据
	mix = mix[:len(mix)/4]

	// 将长度为8的mix分散到32位的digest中去
	digest := make([]byte, common.HashLength)
	for i, val := range mix {
		binary.LittleEndian.PutUint32(digest[i*4:], val)
	}
	return digest, crypto.Keccak256(append(seed, digest...))
}
```

在hashimoto函数中涉及到一个fnv函数，这是一个哈谢算法，全名Fowler-Noll-Vo哈希算法，是以三位发明人名字命名的。FNV能快速hash大量数据并保持较小的冲突率，它的高度分散使它适用于hash一些非常相近的字符串，比如URL，hostname，文件名，text，IP地址等。

```
// fnv is an algorithm inspired by the FNV hash, which in some cases is used as
// a non-associative substitute for XOR. Note that we multiply the prime with
// the full 32-bit input, in contrast with the FNV-1 spec which multiplies the
// prime with one byte (octet) in turn.
// fnv算法
func fnv(a, b uint32) uint32 {
	return a*0x01000193 ^ b
}

// fnvHash mixes in data into mix using the ethash fnv method.
// fnv哈希算法
func fnvHash(mix []uint32, data []uint32) {
	for i := 0; i < len(mix); i++ {
		mix[i] = mix[i]*0x01000193 ^ data[i]
	}
}
```

至此，ethash算法源码就分析完了。

## clique算法

### 原理
Clique算法又称Proof-of-Authortiy(PoA)，是以太坊测试网Ropsten在经过一次DDos攻击之后，数家公司共同研究推出的共识引擎，它运行在以太坊测试网Kovan上。

PoA共识的主要特点：

- PoA是依靠预设好的授权节点(signers)，负责产生block.
- 可以由已授权的signer选举(投票超过50%)加入新的signer。
- 即使存在恶意signer,他最多只能攻击连续块(数量是 (SIGNER_COUNT / 2) + 1) 中的1个,期间可以由其他signer投票踢出该恶意signer。
- 可指定产生block的时间。

Clique原理同样可以用一个公式来表示：

#### n = F(pr, h)

其中，F()是一个数字签名函数(目前是ECDSA)，pr是公钥(common.Address类型)，h是被签名的内容(common.Hash类型)，n是最后生成的签名(一个65bytes的字符串)。

在Clique算法中，所有节点被分为两类:

-  认证节点，类似于矿工节点
- 非认证节点， 类似普通的只能同步的节点

这两种节点的角色可以互换，这种互换是通过投票机制完成的。

- 任何节点都可以参与投票

- 一个节点A只能给节点B投一张票

- 一张投票包括：投票节点地址，被投票节点地址，被投票节点认证状态

- 每进入一个新的epoch，所有之前的pending投票都作废

PoA的大概流程是这样的：

 - __1.创世区块中指定一组认证节点(signers), signer地址保存在区块Extra字段__

- __2.开始mining后,初始指定的signers对block进行签名和广播，签名结果保存在Extra字段__

- __3.由于可能发生signers改变，Extra字段更新已授权的signers__

- __4.每一个高度上处于IN-TURN状态的signer签名的Block优先广播，OUT-OF-TURN状态的随机延时一段时间再进行广播__

- __5.新的signer通过API接口发起proposal，该proposal复用blockHeader的coinbase字段(signerAddress)和nonce字段(0xffffffffffffffff)广播，原有signers对该proposal进行投票，票数过半该发起者成为真正的signer__

- __6.如果需要踢出一个signer，所有signers对该踢出行为进行投票，同样票数过半，该signer变成普通节点__

这里通过API接口发起proposal时对blockHeader字段的复用情况是这样的：

- __针对genesisBlock extra字段包含:__
    - 32bytes前缀(extraVanity)
    - 所有认证用户signers地址
    - 65bytes后缀(extraSeal):用来保存signer签名

- __针对其他Block:__
    - extra: extraVanity和extraSeal
    - Noce:添加signer(nonceAuthVote: 0xffffffffffffffff);移除signer(nonceDropVote: 0x0000000000000000)
    - coinbase:被投票节点地址
    - Difficulty:1--本block签名者(IN-TURN);2-非本block签名者(OUT-OF-TURN)

### 开撸源码

首先还是来看clique共识中几个重要的数据结构。首先看clique自身：

```
// Clique is the proof-of-authority consensus engine proposed to support the
// Ethereum testnet following the Ropsten attacks.
// Clique是在Ropsten攻击之后支持Ethereum testnet的权威证明共识引擎
type Clique struct {
	// 共识引擎配置
	config *params.CliqueConfig // Consensus engine configuration parameters
	// 用于存取检索点快照的数据库
	db     ethdb.Database       // Database to store and retrieve snapshot checkpoints

	// 最近区块的快照，用于加速快照重组
	recents    *lru.ARCCache // Snapshots for recent block to speed up reorgs
	// 最近区块的签名，用于加速挖矿
	signatures *lru.ARCCache // Signatures of recent blocks to speed up mining

	// 当前提出的proposals列表
	proposals map[common.Address]bool // Current list of proposals we are pushing

	// signer地址
	signer common.Address // Ethereum address of the signing key
	// 签名函数
	signFn SignerFn       // Signer function to authorize hashes with
	// 读写锁
	lock   sync.RWMutex   // Protects the signer fields
}
```

共识引擎配置的结构：

```

// CliqueConfig is the consensus engine configs for proof-of-authority based sealing.
// Clique共识引擎配置
type CliqueConfig struct {
	// 距离上一区块出块后的时间间隔(s)
	Period uint64 `json:"period"` // Number of seconds between blocks to enforce
	// 重置投票和检查点的epoch长度
	Epoch  uint64 `json:"epoch"`  // Epoch length to reset votes and checkpoint
}
```

snapshot是指定时间点的投票状态。

```
// Snapshot is the state of the authorization voting at a given point in time.
// 指定时间点的投票状态
type Snapshot struct {
	// clique共识配置
	config   *params.CliqueConfig // Consensus engine parameters to fine tune behavior
	// 最近区块签名的缓存，为了加速恢复
	sigcache *lru.ARCCache        // Cache of recent block signatures to speed up ecrecover

	// 快照建立的区块号
	Number  uint64                      `json:"number"`  // Block number where the snapshot was created
	// 区块hash
	Hash    common.Hash                 `json:"hash"`    // Block hash where the snapshot was created
	// 当下Signer的集合
	Signers map[common.Address]struct{} `json:"signers"` // Set of authorized signers at this moment
	// 最近签名区块地址的集合
	Recents map[uint64]common.Address   `json:"recents"` // Set of recent signers for spam protections
	// 按顺序排列的投票列表
	Votes   []*Vote                     `json:"votes"`   // List of votes cast in chronological order
	// 当前投票结果，可以避免重新计算
	Tally   map[common.Address]Tally    `json:"tally"`   // Current vote tally to avoid recalculating
}
```

其中，Vote是授权签名者为修改授权列表而进行的单一投票；Tally是Tally是一个简单的投票结果，以保持当前的投票得分。

```
// Vote represents a single vote that an authorized signer made to modify the
// list of authorizations.
// 授权签名者为修改授权列表而进行的单一投票
type Vote struct {
	// 提出投票的signer
	Signer    common.Address `json:"signer"`    // Authorized signer that cast this vote
	// 投票所在的区块编号
	Block     uint64         `json:"block"`     // Block number the vote was cast in (expire old votes)
	// 被投票更改认证状态的地址
	Address   common.Address `json:"address"`   // Account being voted on to change its authorization
	// 是否授权或取消对已投票帐户的授权
	Authorize bool           `json:"authorize"` // Whether to authorize or deauthorize the voted account
}

// Tally is a simple vote tally to keep the current score of votes. Votes that
// go against the proposal aren't counted since it's equivalent to not voting.
// 一个简单的投票结果，以保持当前的投票得分。
type Tally struct {
	// 投票是关于授权新的signer还是踢掉signer
	Authorize bool `json:"authorize"` // Whether the vote is about authorizing or kicking someone
	// 到目前为止希望通过提案的投票数
	Votes     int  `json:"votes"`     // Number of votes until now wanting to pass the proposal
}
```

接下来，同样进入worker.agent.engine.seal()代码来看看PoA共识的实现逻辑。

```
// Seal implements consensus.Engine, attempting to create a sealed block using
// the local signing credentials.
// 实现consensus.Engine
func (c *Clique) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	header := block.Header()

	// Sealing the genesis block is not supported
	number := header.Number.Uint64()
	// 当前区块是创世区块
	if number == 0 {
		return nil, errUnknownBlock
	}
	// For 0-period chains, refuse to seal empty blocks (no reward but would spin sealing)
	// 不支持0-period的链
	if c.config.Period == 0 && len(block.Transactions()) == 0 {
		return nil, errWaitTransactions
	}
	// Don't hold the signer fields for the entire sealing procedure
	// 不要在整个签名过程中持有签名字段
	c.lock.RLock()
	// 获取signer和签名方法
	signer, signFn := c.signer, c.signFn
	c.lock.RUnlock()

	// Bail out if we're unauthorized to sign a block
	// 获取快照
	snap, err := c.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return nil, err
	}
	if _, authorized := snap.Signers[signer]; !authorized {
		return nil, errUnauthorized
	}
	// If we're amongst the recent signers, wait for the next block
	// 如果是最近的signers中的一员，等待下一个块
	for seen, recent := range snap.Recents {
		if recent == signer {
			// Signer is among recents, only wait if the current block doesn't shift it out
			// 当前区块没有踢出Signer则继续等待
			if limit := uint64(len(snap.Signers)/2 + 1); number < limit || seen > number-limit {
				log.Info("Signed recently, must wait for others")
				<-stop
				return nil, nil
			}
		}
	}
	// Sweet, the protocol permits us to sign the block, wait for our time
	// 执行到这说明协议允许我们来签名这个区块
	delay := time.Unix(header.Time.Int64(), 0).Sub(time.Now()) // nolint: gosimple
	if header.Difficulty.Cmp(diffNoTurn) == 0 {
		// It's not our turn explicitly to sign, delay it a bit
		// 当前处于OUT-OF-TURN状态,随机一定时间延迟处理
		wiggle := time.Duration(len(snap.Signers)/2+1) * wiggleTime
		delay += time.Duration(rand.Int63n(int64(wiggle)))

		log.Trace("Out-of-turn signing requested", "wiggle", common.PrettyDuration(wiggle))
	}
	log.Trace("Waiting for slot to sign and propagate", "delay", common.PrettyDuration(delay))

	select {
	case <-stop:
		// 停止信号
		return nil, nil
	case <-time.After(delay):
	}
	// Sign all the things!
	// 签名工作
	sighash, err := signFn(accounts.Account{Address: signer}, sigHash(header).Bytes())
	if err != nil {
		return nil, err
	}
	// 更新区块头的extra字段
	copy(header.Extra[len(header.Extra)-extraSeal:], sighash)

	// 通过区块头组装新区块
	return block.WithSeal(header), nil
}
```

其中获取投票状态快照的方法为：

```
// snapshot retrieves the authorization snapshot at a given point in time.
// 获取投票状态快照
func (c *Clique) snapshot(chain consensus.ChainReader, number uint64, hash common.Hash, parents []*types.Header) (*Snapshot, error) {
	// Search for a snapshot in memory or on disk for checkpoints
	// 在内存或磁盘上检索一个快照以检查检查点
	var (
		// 区块头
		headers []*types.Header
		// 快照对象
		snap    *Snapshot
	)
	for snap == nil {
		// If an in-memory snapshot was found, use that
		// 如果一个内存里的快照被找到
		if s, ok := c.recents.Get(hash); ok {
			snap = s.(*Snapshot)
			break
		}
		// If an on-disk checkpoint snapshot can be found, use that
		// 如果一个磁盘检查点的快照被找到
		if number%checkpointInterval == 0 {
			// 从数据库中加载一个快照
			if s, err := loadSnapshot(c.config, c.signatures, c.db, hash); err == nil {
				log.Trace("Loaded voting snapshot from disk", "number", number, "hash", hash)
				snap = s
				break
			}
		}
		// If we're at block zero, make a snapshot
		// 处于创世区块，创建一个快照
		if number == 0 {
			genesis := chain.GetHeaderByNumber(0)
			if err := c.VerifyHeader(chain, genesis, false); err != nil {
				return nil, err
			}
			signers := make([]common.Address, (len(genesis.Extra)-extraVanity-extraSeal)/common.AddressLength)
			for i := 0; i < len(signers); i++ {
				copy(signers[i][:], genesis.Extra[extraVanity+i*common.AddressLength:])
			}
			// 创建新快照
			snap = newSnapshot(c.config, c.signatures, 0, genesis.Hash(), signers)
			if err := snap.store(c.db); err != nil {
				return nil, err
			}
			log.Trace("Stored genesis voting snapshot to disk")
			break
		}
		// No snapshot for this header, gather the header and move backward
		// 没有这个区块头的快照，则收集区块头并向后移动
		var header *types.Header
		if len(parents) > 0 {
			// If we have explicit parents, pick from there (enforced)
			// 如果有明确的父块，必须选出一个
			header = parents[len(parents)-1]
			if header.Hash() != hash || header.Number.Uint64() != number {
				return nil, consensus.ErrUnknownAncestor
			}
			parents = parents[:len(parents)-1]
		} else {
			// No explicit parents (or no more left), reach out to the database
			// 没有明确的父块
			header = chain.GetHeader(hash, number)
			if header == nil {
				return nil, consensus.ErrUnknownAncestor
			}
		}
		headers = append(headers, header)
		number, hash = number-1, header.ParentHash
	}
	// Previous snapshot found, apply any pending headers on top of it
	// 找到之前的快照，将所有pending的区块头放在它的前面
	for i := 0; i < len(headers)/2; i++ {
		headers[i], headers[len(headers)-1-i] = headers[len(headers)-1-i], headers[i]
	}
	// 通过区块头生成新的快照
	snap, err := snap.apply(headers)
	if err != nil {
		return nil, err
	}
	// 将当前快照区块的hash存到recents中
	c.recents.Add(snap.Hash, snap)

	// If we've generated a new checkpoint snapshot, save to disk
	// 如果生成了一个新的检查点快照，保存到磁盘上
	if snap.Number%checkpointInterval == 0 && len(headers) > 0 {
		if err = snap.store(c.db); err != nil {
			return nil, err
		}
		log.Trace("Stored voting snapshot to disk", "number", snap.Number, "hash", snap.Hash)
	}
	return snap, err
}
```

接着继续深入snap.apply函数：

```

// apply creates a new authorization snapshot by applying the given headers to
// the original one.
// 根据区块头创建一个新的signer的快照
func (s *Snapshot) apply(headers []*types.Header) (*Snapshot, error) {
	// Allow passing in no headers for cleaner code
	if len(headers) == 0 {
		return s, nil
	}
	// Sanity check that the headers can be applied
	// 对入参区块头做完整性检查
	for i := 0; i < len(headers)-1; i++ {
		if headers[i+1].Number.Uint64() != headers[i].Number.Uint64()+1 {
			return nil, errInvalidVotingChain
		}
	}

	// 判断区块序号是否连续
	if headers[0].Number.Uint64() != s.Number+1 {
		return nil, errInvalidVotingChain
	}
	// Iterate through the headers and create a new snapshot
	// 遍历区块头数组并建立新的侉子好
	snap := s.copy()

	for _, header := range headers {
		// Remove any votes on checkpoint blocks
		// 移除检查点快照上的任何投票
		number := header.Number.Uint64()
		if number%s.config.Epoch == 0 {
			snap.Votes = nil
			snap.Tally = make(map[common.Address]Tally)
		}
		// Delete the oldest signer from the recent list to allow it signing again
		// 移除投票票数过半，移除signer
		if limit := uint64(len(snap.Signers)/2 + 1); number >= limit {
			delete(snap.Recents, number-limit)
		}
		// Resolve the authorization key and check against signers
		// 解析授权密钥并检查签名者
		signer, err := ecrecover(header, s.sigcache)
		if err != nil {
			return nil, err
		}
		if _, ok := snap.Signers[signer]; !ok {
			return nil, errUnauthorized
		}
		for _, recent := range snap.Recents {
			if recent == signer {
				return nil, errUnauthorized
			}
		}
		// 记录signer为该区块的签名者
		snap.Recents[number] = signer

		// Header authorized, discard any previous votes from the signer
		// 丢弃之前的投票
		for i, vote := range snap.Votes {
			if vote.Signer == signer && vote.Address == header.Coinbase {
				// Uncast the vote from the cached tally
				// 从缓存的计数中取消投票
				snap.uncast(vote.Address, vote.Authorize)

				// Uncast the vote from the chronological list
				// 从时间顺序列表中取消投票
				snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)
				break // only one vote allowed
			}
		}
		// Tally up the new vote from the signer
		// 从签名者那里获得新的投票
		var authorize bool
		switch {
		case bytes.Equal(header.Nonce[:], nonceAuthVote):
			authorize = true
		case bytes.Equal(header.Nonce[:], nonceDropVote):
			authorize = false
		default:
			return nil, errInvalidVote
		}
		if snap.cast(header.Coinbase, authorize) {
			snap.Votes = append(snap.Votes, &Vote{
				Signer:    signer,
				Block:     number,
				Address:   header.Coinbase,
				Authorize: authorize,
			})
		}
		// If the vote passed, update the list of signers
		// 投票通过，更新signers列表
		if tally := snap.Tally[header.Coinbase]; tally.Votes > len(snap.Signers)/2 {

			// 投票是选举新signer
			if tally.Authorize {
				snap.Signers[header.Coinbase] = struct{}{}
			} else {
				// 投票是选移除signer
				delete(snap.Signers, header.Coinbase)

				// Signer list shrunk, delete any leftover recent caches
				// 签名者列表缩小，删除任何剩余的最近缓存
				if limit := uint64(len(snap.Signers)/2 + 1); number >= limit {
					delete(snap.Recents, number-limit)
				}
				// Discard any previous votes the deauthorized signer cast
				// 放弃任何以前的授权签名者投票
				for i := 0; i < len(snap.Votes); i++ {
					if snap.Votes[i].Signer == header.Coinbase {
						// Uncast the vote from the cached tally
						snap.uncast(snap.Votes[i].Address, snap.Votes[i].Authorize)

						// Uncast the vote from the chronological list
						snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)

						i--
					}
				}
			}
			// Discard any previous votes around the just changed account
			// 放弃已更改授权状态的账户之前的投票
			for i := 0; i < len(snap.Votes); i++ {
				if snap.Votes[i].Address == header.Coinbase {
					snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)
					i--
				}
			}
			delete(snap.Tally, header.Coinbase)
		}
	}
	snap.Number += uint64(len(headers))
	snap.Hash = headers[len(headers)-1].Hash()

	return snap, nil
}
```

综上分析，只有认证节点才有权利出块，其他节点只能同步区块。每次出块时，都会创建一个snapshot快照来表示当前时间的投票状态，这里涉及到了基于投票的认证节点的维护机制。每次认证节点的改变都是通过api向外暴露的propose接口，然后所有的认证节点signers对该提议propose进行投票，超过半数通过投票，最后更新认证节点signer列表并将认证住状态发生改变的账户之前的投票做相应处理。

至此，以太坊有关挖矿逻辑的代码，以及两种共识引擎clique和ethash的实现源码就分析完毕了。累屎宝宝了，这篇源码分析真❤️累啊。。。




























