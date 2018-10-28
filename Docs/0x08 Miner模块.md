# 0x08 Miner模块

我们都知道从比特币开始，我们将打包出一个合法区块的节点叫做Miner(矿工)，同时将这个过程叫做Mining(挖矿)。这个比喻是很贴切的，因为无论是Bitcoin还是Eth其代币数量都是有限的，就像地球上的黄金储备量，你从金矿挖出一点其储备就会少一点。

[wiki](https://github.com/ethereum/wiki/wiki/Mining)关于挖矿的描述这里就不再赘述。我们直捣黄龙开撸源码！！！

# Miner结构

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

挖矿逻辑的源码就看完了，明天来继续看看关于agent挖矿到底是怎么实现共识的(ethash算法)。

























