# 0x09 ethash和clique共识引擎

# Consensus以太坊共识引擎

之前讲了以太坊的挖矿逻辑，还没真正涉及到POW的实现。上面也讲到过以太坊的共识引擎有两种，ethash和clique。ethash本质上就是一种POW算法，clique则是POA(ProofOfAuthortiy)算法。目前以太坊采用的是ethash共识引擎，也就是POW算法。今天分别看下两种共识算法的源码实现。

# Ethash算法

### 原理

抛开其内部实现哈希的算法，其基本原理可以用一个公式表示：

> #### RAND(h, nonce) <= M / d

这里h表示区块头的哈希，nonce表示一个自增的变量，RAND表示经过一系列算法生成一个类似随机数的函数。
M表示一个极大的数，d则是当前区块的难度值header.diffculty。

但是涉及到ethash内部哈希值的计算方式就不得不提以下几个概念。

#### DAG
说到这想起一个题外话，之前看到网上有人问比特币矿机能用来挖以太币吗。怎么说呢，有钱任性的话也是可以的，因为这样做的结果是入不敷出。这是为什么呢？

Bitcoin的POW是完全基于算力的，而Ethash则是基于内存和算力的，它和计算机的内存大小和内存带宽正相关。计算能力再强，它每次读取内存的带宽是有限的，这就是为什么即使用来昂贵的ASIC矿机来挖以太币，收益也不会比PC号多少。但是 ，道高一尺魔高一丈，据说比特大陆已经研发出用于以太坊的专业矿机，不过价格不菲每台800美元。题外话就说到这，接着回归正题。

我们知道POW算法依赖一个nonce值输入算法来得到一个低于困难度阈值的哈希值，Eth引入了**[DAG](https://github.com/ethereum/wiki/wiki/Ethash-DAG)**来提供一个大的，瞬态，依赖于块头哈希和Nonce随机生成的固定资源的子集来参与最终哈希值的计算。

DAG资源大约占用1GB大小的内存。 其文件存储路径为:

- Mac/Linux：$(HOME)/.ethash/full-R<REVISION>-<SEEDHASH>

- Windows: $(HOME)/Appdata/Local/Ethash/full-R<REVISION>-<SEEDHASH>

其文件目录命名的意义为：

- <REVISION>：一个十进制整数，当前的修订版本
- <SEEDHASH> ：16个小写的十六进制数字，指定了当前epoch(在以太坊中每30000个区块会生成一个DAG，这个DAG被称为epoch，大约5.2d，125h)种子哈希的前8个字节

DAG文件的内部格式:

- 1.little-endian小端模式存储(每个unint32以little-endian格式编码)

- 2.headerBytes：8-byte magic number(0xfee1deadbaddcafe)

- 3.DAG是一个uint32s类型的二维数组，维度是N * 16，N是一个幻数(从16777186开始)

- 4.DAG的行应按顺序写入文件，行之间没有分隔符

下图便是我Mac上的几个DAG文件：

![DAG路径](https://upload-images.jianshu.io/upload_images/830585-43a196f0aef1003e.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

ethash依赖于DAG实现POW，从[mining的wiki](https://github.com/ethereum/wiki/wiki/Mining#ethash-dag)中得知：DAG需要很长时间才能生成。如果客户端仅按需生成它，可能会在找到新纪元的第一个块之前看到每个纪元转换的漫长等待。一般会预先计算DAG来避免在每个epoch过渡时发生过长的等待时间。geth执行自动的DAG生成，每次维持两个DAG来保障epoch过渡流畅。

#### Dagger-Hashimoto
ethash算法，又叫Dashimoto (Dagger-Hashimoto)，是Hashimoto算法结合Dagger之后产成的一个变种。其实，在最开始以太坊使用的POW就叫[Dagger-Hashimoto](https://github.com/ethereum/wiki/wiki/Dagger-Hashimoto)。后来，进一步改进后便产生了被命名为[ethash](https://github.com/ethereum/wiki/wiki/Ethash)的Dashimoto算法最新版本。这里暂且以最新的ethash为例去分析，旧版本有兴趣的可以自己看看。


[ethash设计的初衷](https://github.com/ethereum/wiki/wiki/Ethash-Design-Rationale)旨在实现以下目标:

- __1.IO饱和度__:算法应消耗几乎所有可用的内存访问带宽(有效的抵抗ASIC)

- __2.GPU友好__:尽可能地使用GPU进行挖矿。

- __3.轻客户端可验证性__:轻客户端应该能够在0.01秒内验证一轮挖掘的有效性，Python或Javascript中验证不到0.1秒，最多1 MB内存（但指数增加）

- __4.轻客户端减速__:使用轻客户端运行算法的过程应该比使用完整客户端的过程慢得多，以至于轻客户端算法不是经济上可行的实现挖掘实现的途径，包括通过专用硬件

- __5.轻客户端快速启动__:轻客户端应该能够完全运行并能够在40秒内在Javascript中验证块

ethash的大概流程是这样的：

- __1.先根据block number以及block header计算出一个种子值seed__

- 2.__使用seed产生一个16MB的伪随机数集cache__

- 3.__根据cache生成一个1GB的数据集DAG(可以根据cache快速定位DAG中指定位置的元素，所以一般轻客户端保存cache，完整客户端保存DAG)__

- 4.__从DAG中随机选择元素对其进行hash，然后判断哈希值是否小于给定值__

- 5.__cache和DAG每个周期(1000个块)更新一次。DAG从1GB开始随着时间线性增长，现在好像达到20多GB了__

### 源码撸起来

[wiki](https://github.com/ethereum/wiki/wiki/Ethash)里关于ethash代码的分析是以Python为例的，这里我们看官方go-ethereum里的源码实现。

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

### generateDataset-DAG生成 
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

上面说了，在计算块头和nonce的哈希时需要用到DAG数据集。这时，会首先从磁盘检索对应需要的DAG文件，有则加载，没有的话就新建一个DAG数据集。

```
// dataset tries to retrieve a mining dataset for the specified block number
// by first checking against a list of in-memory datasets, then against DAGs
// stored on disk, and finally generating one if none can be found.
// 在磁盘上找到一个DAG，如果没有则创建
func (ethash *Ethash) dataset(block uint64) *dataset {
	// 计算当前对应的epoch
	epoch := block / epochLength
	currentI, futureI := ethash.datasets.get(epoch)
	current := currentI.(*dataset)

	// Wait for generation finish.
	// 这里有的话会直接加载，没有的话才会真的创建  详见generate代码
	current.generate(ethash.config.DatasetDir, ethash.config.DatasetsOnDisk, ethash.config.PowMode == ModeTest)

	// If we need a new future dataset, now's a good time to regenerate it.
	// 创建一个将来的DAG以保障epoch过渡流畅
	if futureI != nil {
		future := futureI.(*dataset)
		go future.generate(ethash.config.DatasetDir, ethash.config.DatasetsOnDisk, ethash.config.PowMode == ModeTest)
	}

	return current
}
```

这里有个常量epochLength，我们先来看下有关它的定义。因为后面源码用到了许多类似的常量，我们必须清楚他们表示的意义。

```
const (
	// 初始dataset的字节数 1GB
	datasetInitBytes   = 1 << 30 // Bytes in dataset at genesis
	// 每epoch dataset数据集的增长量
	datasetGrowthBytes = 1 << 23 // Dataset growth per epoch
	// 初始cache的字节数 16MB
	cacheInitBytes     = 1 << 24 // Bytes in cache at genesis
	// 每epoch cache的字节数增长
	cacheGrowthBytes   = 1 << 17 // Cache growth per epoch
	// 每个epoch包含的区块数
	epochLength        = 30000   // Blocks per epoch
	// mix位宽
	mixBytes           = 128     // Width of mix
	// hash长度
	hashBytes          = 64      // Hash length in bytes
	// 散列中的32位无符号整数的个数 DAG是个uint32s类型的二维数组，每个uint32s的元素个数为hashWords
	hashWords          = 16      // Number of 32 bit ints in a hash
	// 每个数据集元素datasetItem的父数  从datasetParents个伪随机选择的缓存数据得到一个datasetItem
	datasetParents     = 256     // Number of parents of each dataset element
	// 一次cache生成的循环次数
	cacheRounds        = 3       // Number of rounds in cache production
	// hashimoto循环中的访问次数
	loopAccesses       = 64      // Number of accesses in hashimoto loop
)
```

接着深入到generate函数，这里也可以从代码中看到它在磁盘里的存储路径。

```
// generate ensures that the dataset content is generated before use.
// 确保在使用之前生成DAG数据集
func (d *dataset) generate(dir string, limit int, test bool) {
	d.once.Do(func() {

		//cache和dataset集合(DAG)大小计算
		csize := cacheSize(d.epoch*epochLength + 1)
		dsize := datasetSize(d.epoch*epochLength + 1)
		seed := seedHash(d.epoch*epochLength + 1)
		if test {
			csize = 1024
			dsize = 32 * 1024
		}
		// If we don't store anything on disk, generate and return
		// 目前DAG目录里还不存在DAG文件，则根据cache创建DAG
		if dir == "" {
			cache := make([]uint32, csize/4)
			generateCache(cache, d.epoch, seed)

			d.dataset = make([]uint32, dsize/4)
			generateDataset(d.dataset, d.epoch, cache)
		}
		// Disk storage is needed, this will get fancy
		// 需要磁盘存储
		var endian string
		if !isLittleEndian() {
			endian = ".be"
		}
		path := filepath.Join(dir, fmt.Sprintf("full-R%d-%x%s", algorithmRevision, seed[:8], endian))
		logger := log.New("epoch", d.epoch)

		// We're about to mmap the file, ensure that the mapping is cleaned up when the
		// cache becomes unused.
		runtime.SetFinalizer(d, (*dataset).finalizer)

		// Try to load the file from disk and memory map it
		// 加载DAG文件并将内存映射到它
		var err error
		d.dump, d.mmap, d.dataset, err = memoryMap(path)
		if err == nil {
			logger.Debug("Loaded old ethash dataset from disk")
			return
		}
		logger.Debug("Failed to load old ethash dataset", "err", err)

		// No previous dataset available, create a new dataset file to fill
		// 没有以前的可用数据集，创建要填充的数据集文件
		cache := make([]uint32, csize/4)
		generateCache(cache, d.epoch, seed)

		d.dump, d.mmap, d.dataset, err = memoryMapAndGenerate(path, dsize, func(buffer []uint32) { generateDataset(buffer, d.epoch, cache) })
		if err != nil {
			logger.Error("Failed to generate mapped ethash dataset", "err", err)

			d.dataset = make([]uint32, dsize/2)
			generateDataset(d.dataset, d.epoch, cache)
		}
		// Iterate over all previous instances and delete old ones
		// 迭代更新DAG文件并删除过于老旧的
		for ep := int(d.epoch) - limit; ep >= 0; ep-- {
			seed := seedHash(uint64(ep)*epochLength + 1)
			path := filepath.Join(dir, fmt.Sprintf("full-R%d-%x%s", algorithmRevision, seed[:8], endian))
			os.Remove(path)
		}
	})
}
```

那一个DAG集合具体是怎么生成的呢，这就要看generateDataset的内部代码了。

```
// generateDataset generates the entire ethash dataset for mining.
// This method places the result into dest in machine byte order.
// 创建用于挖掘的DAG数据集，将结果按机器字节顺序存放到dest
func generateDataset(dest []uint32, epoch uint64, cache []uint32) {
	// Print some debug logs to allow analysis on low end devices
	logger := log.New("epoch", epoch)

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)

		logFn := logger.Debug
		if elapsed > 3*time.Second {
			logFn = logger.Info
		}
		logFn("Generated ethash verification cache", "elapsed", common.PrettyDuration(elapsed))
	}()

	// Figure out whether the bytes need to be swapped for the machine
	// 判断是否是小端模式
	swapped := !isLittleEndian()

	// Convert our destination slice to a byte buffer
	// 将dest转化为字节缓冲区dataset
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&dest))
	header.Len *= 4
	header.Cap *= 4
	dataset := *(*[]byte)(unsafe.Pointer(&header))

	// Generate the dataset on many goroutines since it takes a while
	// 我们知道一个dataset庞大到需要占用月1GB空间
	// 所以这里为了加快创建速度，开启多协程来创建它

	// 协程数
	threads := runtime.NumCPU()
	size := uint64(len(dataset))

	// 创建一个等待信号量，目的是为了等所有协程执行完毕再结束主协程
	var pend sync.WaitGroup
	pend.Add(threads)

	var progress uint32
	for i := 0; i < threads; i++ {
		go func(id int) {
			defer pend.Done()

			// Create a hasher to reuse between invocations
			// 一个在调用之间重用的哈希
			keccak512 := makeHasher(sha3.NewKeccak512())

			// Calculate the data segment this thread should generate
			// 计算当前协程应该计算的数据量
			batch := uint32((size + hashBytes*uint64(threads) - 1) / (hashBytes * uint64(threads)))
			first := uint32(id) * batch
			limit := first + batch
			if limit > uint32(size/hashBytes) {
				limit = uint32(size / hashBytes)
			}
			// Calculate the dataset segment
			// 计算dataset数据
			percent := uint32(size / hashBytes / 100)
			for index := first; index < limit; index++ {

				// dataset数据集由若干个datasetItem组成
				item := generateDatasetItem(cache, index, keccak512)

				// 字节序反转，以保证最后按小端模式存储
				if swapped {
					swap(item)
				}
				//将计算好的一个datasetItem添加到dataset
				copy(dataset[index*hashBytes:], item)

				if status := atomic.AddUint32(&progress, 1); status%percent == 0 {
					logger.Info("Generating DAG in progress", "percentage", uint64(status*100)/(size/hashBytes), "elapsed", common.PrettyDuration(time.Since(start)))
				}
			}
		}(i)
	}
	// Wait for all the generators to finish and return
	pend.Wait()
}
...
// generateDatasetItem combines data from 256 pseudorandomly selected cache nodes,
// and hashes that to compute a single dataset node.
// 组合来自256个伪随机选择的缓存节点的数据，用于计算单个数据集节点的哈希值
func generateDatasetItem(cache []uint32, index uint32, keccak512 hasher) []byte {
	// Calculate the number of theoretical rows (we use one buffer nonetheless)
	// 计算理论行数
	rows := uint32(len(cache) / hashWords)

	// Initialize the mix
	mix := make([]byte, hashBytes)

	binary.LittleEndian.PutUint32(mix, cache[(index%rows)*hashWords]^index)
	for i := 1; i < hashWords; i++ {
		binary.LittleEndian.PutUint32(mix[i*4:], cache[(index%rows)*hashWords+uint32(i)])
	}
	keccak512(mix, mix)

	// Convert the mix to uint32s to avoid constant bit shifting
	// 将mix转换为uint32类型，前面讲过DAG是一个uint32s类型的二维数组
	intMix := make([]uint32, hashWords)
	for i := 0; i < len(intMix); i++ {
		intMix[i] = binary.LittleEndian.Uint32(mix[i*4:])
	}
	// fnv it with a lot of random cache nodes based on index
	// fnv运算从256个伪随机选择的缓存节点的数据得到一个datasetItem的值
	for i := uint32(0); i < datasetParents; i++ {
		parent := fnv(index^i, intMix[i%16]) % rows
		fnvHash(intMix, cache[parent*hashWords:])
	}
	// Flatten the uint32 mix into a binary one and return
	// uint32转化成二进制小端格式
	for i, val := range intMix {
		binary.LittleEndian.PutUint32(mix[i*4:], val)
	}
	keccak512(mix, mix)
	return mix
}
```

### Seal()实现ethash

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

在mine()中hashimotoFull()是为nonce值计算pow的算法，我们继续深入到这个函数中。在它的附近我们发现一个hashimotoLight()函数，这两个函数的入参结构相同，内部代码的处理逻辑也相同。不同的是,前者传的是cache数据集，后者是dataset数据集，再结合名字可以猜测hashimotoLight是hashimotoFull的轻量级实现。

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

至此就达到计算一个nonce的pow值得目的了，判断noce合法的条件是它小于一个给定的target。我们知道比特币出块的时间是10min左右，以太坊呢是12s左右。那么以太坊内部是怎么保证这个出块时间不会有太大的波动，这就需要来看看CalcDifficulty()方法。

```
// CalcDifficulty is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time
// given the parent block's time and difficulty.
// 难度调整算法，用来保障出块时间在一个固定值上下波动
func (ethash *Ethash) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	return CalcDifficulty(chain.Config(), time, parent)
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time
// given the parent block's time and difficulty.
// 计算在给定父块时间和难度下当前块应该具有的难度
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header) *big.Int {
	next := new(big.Int).Add(parent.Number, big1)
	switch {
	// 以太坊不同阶段难度调整算法不同
	case config.IsByzantium(next):
		return calcDifficultyByzantium(time, parent)
	case config.IsHomestead(next):
		return calcDifficultyHomestead(time, parent)
	default:
		return calcDifficultyFrontier(time, parent)
	}
}
```

目前以太坊处在Homestead阶段，所以就以calcDifficultyHomestead为例来看看内部调整难度的算法。其难度计算规则见[eip-2](https://github.com/ethereum/EIPs/blob/master/EIPS/eip-2.md/)。

```
// calcDifficultyHomestead is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time given the
// parent block's time and difficulty. The calculation uses the Homestead rules.
// Homestead阶段的区块难度调整算法
func calcDifficultyHomestead(time uint64, parent *types.Header) *big.Int {
	//这里给出了Homestead阶段的当前难度计算规则
	// https://github.com/ethereum/EIPs/blob/master/EIPS/eip-2.md
	// algorithm:
	// diff = (parent_diff +
	//         (parent_diff / 2048 * max(1 - (block_timestamp - parent_timestamp) // 10, -99))
	//        ) + 2^(periodCount - 2)
	// 2^(periodCount - 2)表示指数难度调整组件

	// 当前时间
	bigTime := new(big.Int).SetUint64(time)
	// 父块出块时间
	bigParentTime := new(big.Int).Set(parent.Time)

	// holds intermediate values to make the algo easier to read & audit
	x := new(big.Int)
	y := new(big.Int)

	// 1 - (block_timestamp - parent_timestamp) // 10
	x.Sub(bigTime, bigParentTime)
	x.Div(x, big10)
	x.Sub(big1, x)

	// max(1 - (block_timestamp - parent_timestamp) // 10, -99)
	if x.Cmp(bigMinus99) < 0 {
		x.Set(bigMinus99)
	}
	// (parent_diff + parent_diff // 2048 * max(1 - (block_timestamp - parent_timestamp) // 10, -99))
	y.Div(parent.Difficulty, params.DifficultyBoundDivisor)
	x.Mul(y, x)
	x.Add(parent.Difficulty, x)

	// minimum difficulty can ever be (before exponential factor)
	if x.Cmp(params.MinimumDifficulty) < 0 {
		x.Set(params.MinimumDifficulty)
	}
	// for the exponential factor
	periodCount := new(big.Int).Add(parent.Number, big1)
	periodCount.Div(periodCount, expDiffPeriod)

	// the exponential factor, commonly referred to as "the bomb"
	// diff = diff + 2^(periodCount - 2)
	if periodCount.Cmp(big1) > 0 {
		y.Sub(periodCount, big2)
		y.Exp(big2, y, nil)
		x.Add(x, y)
	}
	return x
}
```

每次进行挖矿难度的计算是在prepare阶段，prepare函数是实现共识引擎的准备阶段。

```
// Prepare implements consensus.Engine, initializing the difficulty field of a
// header to conform to the ethash protocol. The changes are done inline.
// 实现共识引擎的准备阶段
func (ethash *Ethash) Prepare(chain consensus.ChainReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = ethash.CalcDifficulty(chain, header.Time.Uint64(), parent)
	return nil
}
```

至此，ethash算法源码就分析完了。

# clique算法

### 原理
Clique算法又称Proof-of-Authortiy(PoA)，是以太坊测试网Ropsten在经过一次DDos攻击之后，数家公司共同研究推出的共识引擎，它运行在以太坊测试网Kovan上。

PoA共识的主要特点：

- PoA是依靠预设好的授权节点(signers)，负责产生block.
- 可以由已授权的signer选举(投票超过50%)加入新的signer。
- 即使存在恶意signer,他最多只能攻击连续块(数量是 (SIGNER_COUNT / 2) + 1) 中的1个,期间可以由其他signer投票踢出该恶意signer。
- 可指定产生block的时间。

抛开授权节点的选举算法，Clique原理同样可以用一个公式来表示：

#### n = F(pr, h)

其中，F()是一个数字签名函数(目前是ECDSA)，pr是公钥(common.Address类型)，h是被签名的内容(common.Hash类型)，n是最后生成的签名(一个65bytes的字符串)。

在Clique算法中，所有节点被分为两类:

-  认证节点，类似于矿工节点
- 非认证节点， 类似普通的只能同步的节点

这两种节点的角色可以互换，这种互换是通过对proposal的投票完成的。

- 任何节点N都可以提交一个Propose来申请成为signer,然后所有的signers集体投票

- 一个Signer只能给节点N投一张票

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

综上分析，只有认证节点才有权利出块，其他节点只能同步区块。每次出块时，都会创建一个snapshot快照来表示当前时间的投票状态，这里涉及到了基于投票的认证节点的维护机制。

每次认证节点的改变都是通过api向外暴露的propose接口，然后所有的认证节点signers对该提议propose进行投票，超过半数通过投票，最后更新认证节点signer列表并将认证住状态发生改变的账户之前的投票做相应处理。

同样clique也会涉及到出块时间的控制。

```
// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (c *Clique) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	snap, err := c.snapshot(chain, parent.Number.Uint64(), parent.Hash(), nil)
	if err != nil {
		return nil
	}
	return CalcDifficulty(snap, c.signer)
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
// clique共识难度调整算法 当前块的难度基于前一个块和当前的签名者
func CalcDifficulty(snap *Snapshot, signer common.Address) *big.Int {
	if snap.inturn(snap.Number+1, signer) {
		return new(big.Int).Set(diffInTurn)
	}
	return new(big.Int).Set(diffNoTurn)
}
...
// inturn returns if a signer at a given block height is in-turn or not.
// 给定高度块的签名者是否轮流
func (s *Snapshot) inturn(number uint64, signer common.Address) bool {
	signers, offset := s.signers(), 0
	for offset < len(signers) && signers[offset] != signer {
		offset++
	}
	return (number % uint64(len(signers))) == uint64(offset)
}
```

这里的出块难度实际上就是基于签名者的。如果当前区块的签名者是轮流签名的，那么当前signer可以立即签名一个区块(因为已经轮到这个signer签名了)；反之，如果签名者不是轮流的，那么将会随机等待一段时间再签名这个区块(上个区块很可能就是这个singer签名的)。

```
func (c *Clique) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
...
// 执行到这说明协议允许我们来签名这个区块
	delay := time.Unix(header.Time.Int64(), 0).Sub(time.Now()) // nolint: gosimple
    // 出块难度值判断
	if header.Difficulty.Cmp(diffNoTurn) == 0 {
		// It's not our turn explicitly to sign, delay it a bit
		// 当前处于OUT-OF-TURN状态,随机一定时间延迟处理
		wiggle := time.Duration(len(snap.Signers)/2+1) * wiggleTime
		delay += time.Duration(rand.Int63n(int64(wiggle)))

		log.Trace("Out-of-turn signing requested", "wiggle", common.PrettyDuration(wiggle))
	}
...
}
```



至此，以太坊两种共识引擎clique和ethash的实现源码就分析完毕了。



