# 以太坊源码研读0x09 EVM机制 

EVM,Ethereum Virtual Machine，以太坊虚拟机。它是以太坊智能合约的运行环境。我们知道之前我们写简单的智能合约时都需要将solidlity代码编译形成字节码才能够部署到以太坊上。同时在交易模块讲了一笔交易的大概流程，但是对于交易的真正执行并没有涉及到，其实交易的执行也是依赖于EVM。

# 原理

EVM本质上是一个堆栈机器，最直接的功能就是执行 智能合约。关于其定义，[官档](https://solidity.readthedocs.io/en/v0.4.24/introduction-to-smart-contracts.html#index-6)给出的叙述是这样的：

> The Ethereum Virtual Machine or EVM is the runtime environment for smart contracts in Ethereum. It is not only sandboxed but actually completely isolated, which means that code running inside the EVM has no access to network, filesystem or other processes. Smart contracts even have limited access to other smart contracts.
以太坊虚拟机或EVM是以太坊中智能合约的运行时环境。 它不仅仅是沙箱，而且实际上是完全隔离的，这意味着EVM无法访问网络，文件系统或其他进程。 智能合约甚至可以限制其他智能合约的使用。

接着官档介绍了有关EVM的一些诸如Account，Gas等概念的介绍 ，都是之前接触过的在此略过不提。这里着重看一下EVM的存储系统和其他几个重要的概念。

### 存储系统
EVM机器位宽为256位，即32个字节，256位机器字宽不同于我们经常见到主流的64位的机器字宽，这就表明EVM设计上将考虑一套自己的关于操作，数据，逻辑控制的指令编码。目前主流的处理器原生支持的计算数据类型有：8bits整数，16bits整数，32bits整数，64bits整数。

EVM中每个账户有一块持久化内存区称为 存储 。存储是将256位字映射到256位的键值存储区。 在合约中枚举存储是不可能的，且读存储的相对开销很高，修改存储的开销甚至更高。合约只能读写存储区内属于自己的部分。

第二个内存区域称为内存，合约每次调用会获取一块被清除确保没有脏数据的内存。存储器是线性的，可以在字节级读取，但读取限制为256位宽，而写操作可以是8位或256位宽。当访问（读取或写入）先前未访问过的存储器字(字内的任何偏移)时，存储器会按字(256位)进行扩展。扩容会消耗一定的Gas。随着内存的增大，内存成本越高(二次方指数增长)。

EVM不是基于寄存器的，而是基于栈机器，因此所有计算都在栈上执行。栈的容量为1024，每个元素是一个包含256位的字。可以将最顶部的16个元素之一复制到栈顶，或者将最顶层的元素与其下面的16个元素之一交换。所有其他操作只能从栈中取最顶部的两个(或一个或多个，取决于操作)元素进行运算，然后压栈道栈顶。

### Instruction Set指令集

EVM的指令集量应尽量少，以最大限度地避免可能导致共识问题的错误实现。所有的指令都是针对”256位的字（word）”这个基本的数据类型来进行操作。具备常用的算术、位、逻辑和比较操作。也可以做到有条件和无条件跳转。此外，合约可以访问当前区块的相关属性，比如它的编号和时间戳。

### Message Calls消息调用

合约可以通过消息调用的方式来调用其它合约或者发送以太币到非合约账户。消息调用和交易非常类似，它们都有一个源、目标、数据、以太币、gas和返回数据。事实上每个交易都由一个顶层消息调用组成，这个消息调用又可创建更多的消息调用。

合约可以决定在其内部的消息调用中，对于剩余的 gas ，应发送和保留多少。如果在内部消息调用时发生了out-of-gas异常（或其他任何异常），这将由一个被压入栈顶的错误值所指明。此时，只有与该内部消息调用一起发送的gas会被消耗掉。并且，Solidity中，发起调用的合约默认会触发一个手工的异常，以便异常可以从调用栈里“冒泡出来”。

 如前文所述，被调用的合约（可以和调用者是同一个合约）会获得一块刚刚清空过的内存，并可以访问调用的payload——由被称为 calldata 的独立区域所提供的数据。调用执行结束后，返回数据将被存放在调用方预先分配好的一块内存中。 调用深度被 限制 为 1024 ，因此对于更加复杂的操作，我们应 使用循环而不是递归。

### Delegatecall / Callcode and Libraries 委托调用和代码调用库

Delegatecall是EVM中一种特殊的消息调用，它与普通消息调用的区别在于:目标地址的代码将在发起调用的合约的上下文中执行，并且 msg.sender 和 msg.value 不变。这就意味着合约可以在运行时从不同的地址动态加载代码。存储、当前地址和余额都指向发起调用的合约，只有代码是从被调用地址获取的。

如此就使得Solidity实现"库调用"成为可能，于是就出现了可复用的代码调用库。

### Logs日志

Logs是一直能够特殊的可索引的数据结构，其存储的数据可以一直映射到区块层级，Solidity借助它来实现事件(Events)。

智能合约一经创建就无法访问Logs，但Logs可以从区块链外有效地访问。部分Logs数据被存储在Bloom filter(布隆过滤器)中，因此可以以高效且加密的方式搜索此数据，也是因为这样那些没有下载全节点的轻客户端也能够访问这些数据。

### Create & Self-destruct 智能合约的创建和销毁

智能合约甚至可以通过特殊的指令来创建其他合约(并不是简单地调用零地址)。这种创建合约的消息调用和普通消息调用的区别在于，负载会被执行并且执行结果会被存储为合约代码，同时将新合约地址返回给调用者。

智能合约代码从区块链上移除的唯一方式是合约在合约地址上执行自毁操作selfdestruct。存储在合约上的以太币会发送给指定账户，然后从状态中移除存储和代码。尽管一个合约没有显式地调用selfdestruct，它依然可以通过delegatecall或callcode来间接地执行自毁操作。

咳咳…连蒙带猜加上谷译的助攻终于将官档看完了，以上都是个人所读仅供参考以达抛砖引玉之用，大佬们深入理解一切还是要以官档原文为准的。

# 源码撸起来

### 高屋建瓴总览大局

了解几本原理后，就可以从源码入手来分析下EVM的运行机制。首先来看看源码vm相关的目录结构：

```
➜  vm pwd
/Users/chaors/BlockChain/ethereum/SourceCodeRead/go-ethereum-master_read/core/vm
➜  vm tree
.
|____memory.go                    //EVM内存
|____opcodes.go                   //op指令集
|____analysis.go                  //跳转目标判断
|____gas_table_test.go      
|____gas_table.go                 //指令耗费gas计算表
|____evm.go                       //evm对外接口
|____gas.go                       //gas花费计算
|____intpool_test.go
|____logger.go                    //evm日志
|____int_pool_verifier_empty.go
|____runtime
| |____env.go                     //执行环境
| |____runtime.go                 //运行时
| |____runtime_example_test.go
| |____doc.go
| |____runtime_test.go
| |____fuzz.go
|____interface.go
|____analysis_test.go
|____instructions.go              //指令集实现
|____gen_structlog.go
|____contracts.go                 //预编译的合约
|____memory_table.go              //evm内存操作表
|____noop.go
|____instructions_test.go
|____doc.go
|____stack.go                     //栈
|____common.go                    //一些共有方法
|____stack_table.go               //栈验证表
|____interpreter.go               //解释器
|____intpool.go                   //int值存储池
|____jump_table.go                //指令和指令操作对应表
|____contract.go                  //智能合约
|____int_pool_verifier.go
|____contracts_test.go
|____logger_test.go
|____errors.go                    //错误类
```

### EVM结构

```
type Context struct {
	// CanTransfer returns whether the account contains
	// sufficient ether to transfer the value
	// 返回账户是否包含足够的用来传输的以太币
	CanTransfer CanTransferFunc
	// Transfer transfers ether from one account to the other
	// 将以太从一个帐户转移到另一个帐户
	Transfer TransferFunc
	// GetHash returns the hash corresponding to n
	GetHash GetHashFunc

	// Message information
	// 消息相关信息
	Origin   common.Address // Provides information for ORIGIN
	GasPrice *big.Int       // Provides information for GASPRICE

	// Block information
	// 区块相关信息
	Coinbase    common.Address // Provides information for COINBASE
	GasLimit    uint64         // Provides information for GASLIMIT
	BlockNumber *big.Int       // Provides information for NUMBER
	Time        *big.Int       // Provides information for TIME
	Difficulty  *big.Int       // Provides information for DIFFICULTY
}

// EVM is the Ethereum Virtual Machine base object and provides
// the necessary tools to run a contract on the given state with
// the provided context. It should be noted that any error
// generated through any of the calls should be considered a
// revert-state-and-consume-all-gas operation, no checks on
// specific errors should ever be performed. The interpreter makes
// sure that any errors generated are to be considered faulty code.
//
// The EVM should never be reused and is not thread safe.
// // EVM是以太坊虚拟机基础对象，并提供必要的工具，以使用提供的上下文运行给定状态的合约。
// 应该指出的是，任何调用产生的任何错误都应该被认为是一种回滚修改状态和消耗所有GAS操作，
// 不应该执行对具体错误的检查。 解释器确保生成的任何错误都被认为是错误的代码。
type EVM struct {
	// Context provides auxiliary blockchain related information
	// 辅助信息对象(包括GasPrice，GasLimit，BlockNumber等信息)
	Context
	// StateDB gives access to the underlying state
	// 为EVM提供StateDB相关操作
	StateDB StateDB
	// Depth is the current call stack
	// 当前调用的栈
	depth int

	// chainConfig contains information about the current chain
	// 链配置信息
	chainConfig *params.ChainConfig
	// chain rules contains the chain rules for the current epoch
	// 链规则
	chainRules params.Rules
	// virtual machine configuration options used to initialise the
	// evm.
	// 虚拟机配置
	vmConfig Config
	// global (to this context) ethereum virtual machine
	// used throughout the execution of the tx.
	// 解释器
	interpreter *Interpreter
	// abort is used to abort the EVM calling operations
	// NOTE: must be set atomically
	// 用于中止EVM调用操作
	abort int32
	// callGasTemp holds the gas available for the current call. This is needed because the
	// available gas is calculated in gasCall* according to the 63/64 rule and later
	// applied in opCall*.
	// 当前call可用的gas
	callGasTemp uint64
}

// NewEVM returns a new EVM. The returned EVM is not thread safe and should
// only ever be used *once*.
func NewEVM(ctx Context, statedb StateDB, chainConfig *params.ChainConfig, vmConfig Config) *EVM {
	evm := &EVM{
		Context:     ctx,
		StateDB:     statedb,
		vmConfig:    vmConfig,
		chainConfig: chainConfig,
		chainRules:  chainConfig.Rules(ctx.BlockNumber),
	}

	evm.interpreter = NewInterpreter(evm, vmConfig)
	return evm
}
```

### Contract结构
既然EVM最直接的功能就是运行智能合约，接下来就看看智能合约的数据结构。

```
// Contract represents an ethereum contract in the state database. It contains
// the the contract code, calling arguments. Contract implements ContractRef
// 数据库中的以太坊智能合约，包括合约代码和调用参数
type Contract struct {
	// CallerAddress is the result of the caller which initialised this
	// contract. However when the "call method" is delegated this value
	// needs to be initialised to that of the caller's caller.
	// 合约调用者
	CallerAddress common.Address
	caller        ContractRef
	self          ContractRef

	// JUMPDEST分析的结果
	jumpdests destinations // result of JUMPDEST analysis.

	// 合约代码
	Code     []byte
	CodeHash common.Hash
	// 合约地址
	CodeAddr *common.Address
	Input    []byte

	Gas   uint64
	value *big.Int

	Args []byte

	// 是否委托调用
	DelegateCall bool
}

// NewContract returns a new contract environment for the execution of EVM.
// 为EVM创建合约环境
func NewContract(caller ContractRef, object ContractRef, value *big.Int, gas uint64) *Contract {
	c := &Contract{CallerAddress: caller.Address(), caller: caller, self: object, Args: nil}

	if parent, ok := caller.(*Contract); ok {
		// Reuse JUMPDEST analysis from parent context if available.
		c.jumpdests = parent.jumpdests
	} else {
		c.jumpdests = make(destinations)
	}

	// Gas should be a pointer so it can safely be reduced through the run
	// This pointer will be off the state transition
	c.Gas = gas
	// ensures a value is set
	c.value = value

	return c
}
```

### EVM工作逻辑

EVM运行的大概逻辑是这样的：

- __1.创建EVM运行的上下文环境，同时实例化一个EVM对象__

- __2.合约不存在则创建新合约，使用已经存在的合约则世界调用call__

- __3.EVM通过interpreter解释器来执行智能合约__

创建EVM对象的代码：

```
// NewEVMContext creates a new context for use in the EVM.
// 1.创建EVM上下文环境
func NewEVMContext(msg Message, header *types.Header, chain ChainContext, author *common.Address) vm.Context {
	// If we don't have an explicit author (i.e. not mining), extract from the header
	// 如果不挖矿，受益人从区块头中提取
	var beneficiary common.Address
	if author == nil {
		beneficiary, _ = chain.Engine().Author(header) // Ignore error, we're past header validation
	} else {
		beneficiary = *author
	}
	return vm.Context{
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
		GetHash:     GetHashFn(header, chain),
		Origin:      msg.From(),
		Coinbase:    beneficiary,
		BlockNumber: new(big.Int).Set(header.Number),
		Time:        new(big.Int).Set(header.Time),
		Difficulty:  new(big.Int).Set(header.Difficulty),
		GasLimit:    header.GasLimit,
		GasPrice:    new(big.Int).Set(msg.GasPrice()),
	}
}
...
// NewEVM returns a new EVM. The returned EVM is not thread safe and should
// only ever be used *once*.
// 2.创建EVM对象
func NewEVM(ctx Context, statedb StateDB, chainConfig *params.ChainConfig, vmConfig Config) *EVM {
	evm := &EVM{
		Context:     ctx,
		StateDB:     statedb,
		vmConfig:    vmConfig,
		chainConfig: chainConfig,
		chainRules:  chainConfig.Rules(ctx.BlockNumber),
	}

	// 3.创建EVM解释器
	evm.interpreter = NewInterpreter(evm, vmConfig)
	return evm
}
...
// NewInterpreter returns a new instance of the Interpreter.
// 3.创建解释器
func NewInterpreter(evm *EVM, cfg Config) *Interpreter {
	// We use the STOP instruction whether to see
	// the jump table was initialised. If it was not
	// we'll set the default jump table.
	if !cfg.JumpTable[STOP].valid {
		switch {
		case evm.ChainConfig().IsConstantinople(evm.BlockNumber):
			cfg.JumpTable = constantinopleInstructionSet
		case evm.ChainConfig().IsByzantium(evm.BlockNumber):
			cfg.JumpTable = byzantiumInstructionSet
		case evm.ChainConfig().IsHomestead(evm.BlockNumber):
			cfg.JumpTable = homesteadInstructionSet
		default:
			cfg.JumpTable = frontierInstructionSet
		}
	}

	return &Interpreter{
		evm:      evm,
		cfg:      cfg,
		gasTable: evm.ChainConfig().GasTable(evm.BlockNumber),
	}
}
```

创建合约的代码

```
// Create creates a new contract using code as deployment code.
// 创建合约
func (evm *EVM) Create(caller ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {

	// Depth check execution. Fail if we're trying to execute above the
	// limit.
	// 执行深度检查，如果超出设定的深度限制  创建失败
	if evm.depth > int(params.CallCreateDepth) {
		return nil, common.Address{}, gas, ErrDepth
	}
	// 账户余额不足，创建失败
	if !evm.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, common.Address{}, gas, ErrInsufficientBalance
	}
	// Ensure there's no existing contract already at the designated address
	// 确保指定地址没有已存在的相同合约
	nonce := evm.StateDB.GetNonce(caller.Address())
	evm.StateDB.SetNonce(caller.Address(), nonce+1)

	// 创建合约地址
	contractAddr = crypto.CreateAddress(caller.Address(), nonce)
	contractHash := evm.StateDB.GetCodeHash(contractAddr)
	if evm.StateDB.GetNonce(contractAddr) != 0 || (contractHash != (common.Hash{}) && contractHash != emptyCodeHash) {
		return nil, common.Address{}, 0, ErrContractAddressCollision
	}
	// Create a new account on the state
	// 创建数据库快照，为了迅速回滚
	snapshot := evm.StateDB.Snapshot()
	// 在当前状态新建合约账户
	evm.StateDB.CreateAccount(contractAddr)
	if evm.ChainConfig().IsEIP158(evm.BlockNumber) {
		evm.StateDB.SetNonce(contractAddr, 1)
	}
	// 转账操作
	evm.Transfer(evm.StateDB, caller.Address(), contractAddr, value)

	// initialise a new contract and set the code that is to be used by the
	// EVM. The contract is a scoped environment for this execution context
	// only.
	// 创建合约
	contract := NewContract(caller, AccountRef(contractAddr), value, gas)
	// 设置合约代码
	contract.SetCallCode(&contractAddr, crypto.Keccak256Hash(code), code)

	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, contractAddr, gas, nil
	}

	if evm.vmConfig.Debug && evm.depth == 0 {
		evm.vmConfig.Tracer.CaptureStart(caller.Address(), contractAddr, true, code, gas, value)
	}
	start := time.Now()

	// 执行合约的初始化
	ret, err = run(evm, contract, nil)

	// check whether the max code size has been exceeded
	// 检查初始化生成的代码长度是否超过限制
	maxCodeSizeExceeded := evm.ChainConfig().IsEIP158(evm.BlockNumber) && len(ret) > params.MaxCodeSize
	// if the contract creation ran successfully and no errors were returned
	// calculate the gas required to store the code. If the code could not
	// be stored due to not enough gas set an error and let it be handled
	// by the error checking condition below.
	// 合约创建成功
	if err == nil && !maxCodeSizeExceeded {
		// 计算存储代码所需要的Gas
		createDataGas := uint64(len(ret)) * params.CreateDataGas
		if contract.UseGas(createDataGas) {
			evm.StateDB.SetCode(contractAddr, ret)
		} else {
			// 当前拥有的Gas不足以存储代码
			err = ErrCodeStoreOutOfGas
		}
	}

	// When an error was returned by the EVM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in homestead this also counts for code storage gas errors.
	// 合约创建失败，借助上面创建的快照快速回滚
	if maxCodeSizeExceeded || (err != nil && (evm.ChainConfig().IsHomestead(evm.BlockNumber) || err != ErrCodeStoreOutOfGas)) {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != errExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	// Assign err if contract code size exceeds the max while the err is still empty.
	if maxCodeSizeExceeded && err == nil {
		err = errMaxCodeSizeExceeded
	}
	if evm.vmConfig.Debug && evm.depth == 0 {
		evm.vmConfig.Tracer.CaptureEnd(ret, gas-contract.Gas, time.Since(start), err)
	}
	return ret, contractAddr, contract.Gas, err
}
...
// run runs the given contract and takes care of running precompiles with a fallback to the byte code interpreter.
func run(evm *EVM, contract *Contract, input []byte) ([]byte, error) {
	if contract.CodeAddr != nil {
		precompiles := PrecompiledContractsHomestead
		if evm.ChainConfig().IsByzantium(evm.BlockNumber) {
			precompiles = PrecompiledContractsByzantium
		}
		if p := precompiles[*contract.CodeAddr]; p != nil {
			// 运行预编译合约
			return RunPrecompiledContract(p, input, contract)
		}
	}
	// 解释器执行合约代码
	return evm.interpreter.Run(contract, input)
}
```

这里合约代码的执行后续再看，当调用已创建的合约时，使用的是call方法。Call方法和create方法的逻辑大体相同，这里分析下他们的不同之处:

- 1.call调用的是一个已经存在合约账户的合约，create是新建一个合约账户。

- 2.call里evm.Transfer发生在合约的发送方和接收方，create里则是创建合约用户的账户和该合约用户之间。

```
// Call executes the contract associated with the addr with the given input as
// parameters. It also handles any necessary value transfer required and takes
// the necessary steps to create accounts and reverses the state in case of an
// execution error or failed value transfer.
// 使用给定输入作为参数执行与addr关联的合约
func (evm *EVM) Call(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}

	// Fail if we're trying to execute above the call depth limit
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
	// Fail if we're trying to transfer more than the available balance
	if !evm.Context.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, gas, ErrInsufficientBalance
	}

	var (
		to       = AccountRef(addr)
		snapshot = evm.StateDB.Snapshot()
	)
	if !evm.StateDB.Exist(addr) {
		precompiles := PrecompiledContractsHomestead
		if evm.ChainConfig().IsByzantium(evm.BlockNumber) {
			precompiles = PrecompiledContractsByzantium
		}
		if precompiles[addr] == nil && evm.ChainConfig().IsEIP158(evm.BlockNumber) && value.Sign() == 0 {
			// Calling a non existing account, don't do antything, but ping the tracer
			if evm.vmConfig.Debug && evm.depth == 0 {
				evm.vmConfig.Tracer.CaptureStart(caller.Address(), addr, false, input, gas, value)
				evm.vmConfig.Tracer.CaptureEnd(ret, 0, 0, nil)
			}
			return nil, gas, nil
		}
		evm.StateDB.CreateAccount(addr)
	}
	evm.Transfer(evm.StateDB, caller.Address(), to.Address(), value)

	// Initialise a new contract and set the code that is to be used by the EVM.
	// The contract is a scoped environment for this execution context only.
	contract := NewContract(caller, to, value, gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

	start := time.Now()

	// Capture the tracer start/end events in debug mode
	if evm.vmConfig.Debug && evm.depth == 0 {
		evm.vmConfig.Tracer.CaptureStart(caller.Address(), addr, false, input, gas, value)

		defer func() { // Lazy evaluation of the parameters
			evm.vmConfig.Tracer.CaptureEnd(ret, gas-contract.Gas, time.Since(start), err)
		}()
	}
	ret, err = run(evm, contract, input)

	// When an error was returned by the EVM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in homestead this also counts for code storage gas errors.
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != errExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}
```

### DelegateCall

上面阅读官档时，涉及到一个DelegateCall委托调用的概念。上面看的Call函数便是便是普通的消息调用，接下来看看EVM中几个特殊的消息调用。这里只讲它们的特殊之处，代码逻辑和Call大体相同，源码就不再看了，参考Call即可。

- CallCode，它与Call不同的地方在于它使用调用者的EVMContext来执行给定地址的合约代码。

- DelegateCall，它与CallCode不同的地方在于它调用者被设置为调用者的调用者

- StaticCall，它不允许执行任何状态的修改

- 以上三个特殊的消息调用只能由opcode触发，它们不像Call可以由外部调用。

### Interpreter EVM解释器

合约的执行最终是靠解释器Interpreter来实现的，这里就来看看Interpreter的数据结构。

```
// Config are the configuration options for the Interpreter
// 解释器配置类
type Config struct {
	// Debug enabled debugging Interpreter options
	// 启用调试
	Debug bool
	// Tracer is the op code logger
	// 操作码记录器
	Tracer Tracer
	// NoRecursion disabled Interpreter call, callcode,
	// delegate call and create.
	// 禁用解释器调用，代码库调用，委托调用
	NoRecursion bool
	// Enable recording of SHA3/keccak preimages
	// 启用SHA3/keccak
	EnablePreimageRecording bool
	// JumpTable contains the EVM instruction table. This
	// may be left uninitialised and will be set to the default
	// table.
	// 操作码opcode对应的操作表
	JumpTable [256]operation
}

// Interpreter is used to run Ethereum based contracts and will utilise the
// passed environment to query external sources for state information.
// The Interpreter will run the byte code VM based on the passed
// configuration.
// 用来运行智能合约的字节码
type Interpreter struct {
	evm      *EVM
	// 解释器配置
	cfg      Config
	// gas价格表，根据不同的以太坊阶段来决定
	gasTable params.GasTable
	intPool  *intPool

	readOnly   bool   // Whether to throw on stateful modifications
	// 最后一个call调用的返回值
	returnData []byte // Last CALL's return data for subsequent reuse
}
```

接着继续看它是怎么实现智能合约的执行的。

```
// Run loops and evaluates the contract's code with the given input data and returns
// the return byte-slice and an error if one occurred.
//
// It's important to note that any errors returned by the interpreter should be
// considered a revert-and-consume-all-gas operation except for
// errExecutionReverted which means revert-and-keep-gas-left.
// 执行合约代码
func (in *Interpreter) Run(contract *Contract, input []byte) (ret []byte, err error) {
	if in.intPool == nil {
		in.intPool = poolOfIntPools.get()
		defer func() {
			poolOfIntPools.put(in.intPool)
			in.intPool = nil
		}()
	}

	// Increment the call depth which is restricted to 1024
	// 调用深度递增，evm执行栈的深度不能超过1024
	in.evm.depth++
	defer func() { in.evm.depth-- }()

	// Reset the previous call's return data. It's unimportant to preserve the old buffer
	// as every returning call will return new data anyway.
	// 重置上一个call的返回数据
	in.returnData = nil

	// Don't bother with the execution if there's no code.
	// 合约代码为空
	if len(contract.Code) == 0 {
		return nil, nil
	}

	var (
		op    OpCode        // current opcode
		mem   = NewMemory() // bound memory
		stack = newstack()  // local stack
		// For optimisation reason we're using uint64 as the program counter.
		// It's theoretically possible to go above 2^64. The YP defines the PC
		// to be uint256. Practically much less so feasible.
		pc   = uint64(0) // program counter
		cost uint64
		// copies used by tracer
		pcCopy  uint64 // needed for the deferred Tracer
		gasCopy uint64 // for Tracer to log gas remaining before execution
		logged  bool   // deferred Tracer should ignore already logged steps
	)
	contract.Input = input

	// Reclaim the stack as an int pool when the execution stops
	// 执行停止时将栈回收为int值缓存池
	defer func() { in.intPool.put(stack.data...) }()

	if in.cfg.Debug {
		defer func() {
			if err != nil {
				if !logged {
					in.cfg.Tracer.CaptureState(in.evm, pcCopy, op, gasCopy, cost, mem, stack, contract, in.evm.depth, err)
				} else {
					in.cfg.Tracer.CaptureFault(in.evm, pcCopy, op, gasCopy, cost, mem, stack, contract, in.evm.depth, err)
				}
			}
		}()
	}
	// The Interpreter main run loop (contextual). This loop runs until either an
	// explicit STOP, RETURN or SELFDESTRUCT is executed, an error occurred during
	// the execution of one of the operations or until the done flag is set by the
	// parent context.
	// 解释器主循环，循环运行直到执行显式STOP，RETURN或SELFDESTRUCT，发生错误
	for atomic.LoadInt32(&in.evm.abort) == 0 {
		if in.cfg.Debug {
			// Capture pre-execution values for tracing.
			// 捕获预执行的值进行跟踪
			logged, pcCopy, gasCopy = false, pc, contract.Gas
		}

		// Get the operation from the jump table and validate the stack to ensure there are
		// enough stack items available to perform the operation.
		// 从合约的二进制数据i获取第pc个opcode操作符 opcode是以太坊虚拟机指令，一共不超过256个，正好一个byte大小能装下
		op = contract.GetOp(pc)
		// 从JumpTable表中查询op对应的操作
		operation := in.cfg.JumpTable[op]
		if !operation.valid {
			return nil, fmt.Errorf("invalid opcode 0x%x", int(op))
		}
		if err := operation.validateStack(stack); err != nil {
			return nil, err
		}
		// If the operation is valid, enforce and write restrictions
		// 操作有效，强制执行
		if err := in.enforceRestrictions(op, operation, stack); err != nil {
			return nil, err
		}

		var memorySize uint64
		// calculate the new memory size and expand the memory to fit
		// the operation
		// 计算新的内存大小以适应操作，必要时进行扩容
		if operation.memorySize != nil {
			// memSize不能大于64位
			memSize, overflow := bigUint64(operation.memorySize(stack))
			if overflow {
				return nil, errGasUintOverflow
			}
			// memory is expanded in words of 32 bytes. Gas
			// is also calculated in words.
			// 扩容按32字节的字扩展
			if memorySize, overflow = math.SafeMul(toWordSize(memSize), 32); overflow {
				return nil, errGasUintOverflow
			}
		}
		// consume the gas and return an error if not enough gas is available.
		// cost is explicitly set so that the capture state defer method can get the proper cost
		// 计算执行操作所需要的gas
		cost, err = operation.gasCost(in.gasTable, in.evm, contract, stack, mem, memorySize)
		// gas不足
		if err != nil || !contract.UseGas(cost) {
			return nil, ErrOutOfGas
		}
		if memorySize > 0 {
			mem.Resize(memorySize)
		}

		if in.cfg.Debug {
			in.cfg.Tracer.CaptureState(in.evm, pc, op, gasCopy, cost, mem, stack, contract, in.evm.depth, err)
			logged = true
		}

		// execute the operation
		// 执行操作
		res, err := operation.execute(&pc, in.evm, contract, mem, stack)
		// verifyPool is a build flag. Pool verification makes sure the integrity
		// of the integer pool by comparing values to a default value.
		// 验证int值缓存池
		if verifyPool {
			verifyIntegerPool(in.intPool)
		}
		// if the operation clears the return data (e.g. it has returning data)
		// set the last return to the result of the operation.
		// 将最后一次返回设为操作结果
		if operation.returns {
			in.returnData = res
		}

		switch {
		case err != nil:
			return nil, err
		case operation.reverts:
			return res, errExecutionReverted
		case operation.halts:
			return res, nil
		case !operation.jumps:
			pc++
		}
	}
	return nil, nil
}
```

### JumpTable(opCode-operation)

在执行合约的时候涉及到contract.GetOp(pc)方法从合约二进制代码中取出第pc个操作符opcode，然后再按对应关系找到opcode对应的操作operation。这里的对应关系就保存在jump_table中。

这里先要理解操作符opcode的概念，它是EVM的操作符。通俗地讲，一个opcode就是一个byte，solidity合约编译形成的bytecode中，一个byte就代表一个opcode。opcodes.go中定义了所有的操作符，并将所有的操作符按功能分类。例如下面是一组块操作相关的操作符：

```
// 0x40 range - block operations.
const (
	BLOCKHASH OpCode = 0x40 + iota
	COINBASE
	TIMESTAMP
	NUMBER
	DIFFICULTY
	GASLIMIT
)
```

每一个opcode都会对应一个具体的操作operation，一个操作包含其操作函数以及一些必要的参数。

```
type operation struct {
	// execute is the operation function
	// 操作函数
	execute executionFunc
	// gasCost is the gas function and returns the gas required for execution
	// 计算操作需要多少gas的函数
	gasCost gasFunc
	// validateStack validates the stack (size) for the operation
	// 验证操作的栈
	validateStack stackValidationFunc
	// memorySize returns the memory size required for the operation
	// 操作需要的内存大小
	memorySize memorySizeFunc

	// 操作终止
	halts   bool // indicates whether the operation should halt further execution
	// 操作跳转
	jumps   bool // indicates whether the program counter should not increment
	// 是否写入
	writes  bool // determines whether this a state modifying operation
	// 操作是否有效
	valid   bool // indication whether the retrieved operation is valid and known
	// 出错回滚
	reverts bool // determines whether the operation reverts state (implicitly halts)
	// 操作返回
	returns bool // determines whether the operations sets the return data content
}
```

opcode和operation的对应关系都在jump_table.go中。例如我们上面举例的相关块操作的操作符，这里以EXTCODECOPY(0x3d)操作符为例：

```
EXTCODECOPY: {
			execute:       opExtCodeCopy,
			gasCost:       gasExtCodeCopy,
			validateStack: makeStackFunc(4, 0),
			memorySize:    memoryExtCodeCopy,
			valid:         true,
		},
```

针对每一个具体的操作operation，其内部属性对应的实现代码为：

- execute---instructions.go，例如上面🌰里的opExtCodeCopy

- gasCost---gas_table.go, 例如上面🌰里的gasExtCodeCopy

- validateStack---stack_table,例如上面🌰里的makeStackFunc

- memorySize---memory_table.go,例如上面🌰里的memoryExtCodeCopy

### Stack栈

EVM是基于栈的虚拟机，这里栈的作用是用来保存操作数的。

```
// Stack is an object for basic stack operations. Items popped to the stack are
// expected to be changed and modified. stack does not take care of adding newly
// initialised objects.
type Stack struct {
	data []*big.Int
}

func newstack() *Stack {
	return &Stack{data: make([]*big.Int, 0, 1024)}
}

func (st *Stack) push(d *big.Int) {
	// NOTE push limit (1024) is checked in baseCheck
	//stackItem := new(big.Int).Set(d)
	//st.data = append(st.data, stackItem)
	st.data = append(st.data, d)
}

func (st *Stack) pop() (ret *big.Int) {
	ret = st.data[len(st.data)-1]
	st.data = st.data[:len(st.data)-1]
	return
}
```

### Memory & stateDB

Memery类为EVM实现了一个简单的内存模型。它主要在执行合约时针对operation进行一些内存里的参数拷贝。

```
// Memory implements a simple memory model for the ethereum virtual machine.
type Memory struct {
	// 内存
	store       []byte
	// 最后一次的gas花费
	lastGasCost uint64
}

// NewMemory returns a new memory memory model.
func NewMemory() *Memory {
	return &Memory{}
}
```

前面在创建合约账户的时候，将合约代码存储到了数据库。当创建合约失败的时候，也是利用数据库快照进行回滚状态的。

### 🌰

当有这样一段智能合约代码:

```
pragma solidity ^0.4.0;
contract SimpleStorage {
    uint storedData;

    function set(uint x) public {
        storedData = x;
    }

    function get() public returns (uint) {
        return storedData;
    }
}
```

在Remix编译器进行编译后得到字节码:

```
{
	"object": "606060405260a18060106000396000f360606040526000357c01000000000000000000000000000000000000000000000000000000009004806360fe47b11460435780636d4ce63c14605d57603f565b6002565b34600257605b60048080359060200190919050506082565b005b34600257606c60048050506090565b6040518082815260200191505060405180910390f35b806000600050819055505b50565b60006000600050549050609e565b9056",
	"opcodes": "PUSH1 0x60 PUSH1 0x40 MSTORE PUSH1 0xA1 DUP1 PUSH1 0x10 PUSH1 0x0 CODECOPY PUSH1 0x0 RETURN PUSH1 0x60 PUSH1 0x40 MSTORE PUSH1 0x0 CALLDATALOAD PUSH29 0x100000000000000000000000000000000000000000000000000000000 SWAP1 DIV DUP1 PUSH4 0x60FE47B1 EQ PUSH1 0x43 JUMPI DUP1 PUSH4 0x6D4CE63C EQ PUSH1 0x5D JUMPI PUSH1 0x3F JUMP JUMPDEST PUSH1 0x2 JUMP JUMPDEST CALLVALUE PUSH1 0x2 JUMPI PUSH1 0x5B PUSH1 0x4 DUP1 DUP1 CALLDATALOAD SWAP1 PUSH1 0x20 ADD SWAP1 SWAP2 SWAP1 POP POP PUSH1 0x82 JUMP JUMPDEST STOP JUMPDEST CALLVALUE PUSH1 0x2 JUMPI PUSH1 0x6C PUSH1 0x4 DUP1 POP POP PUSH1 0x90 JUMP JUMPDEST PUSH1 0x40 MLOAD DUP1 DUP3 DUP2 MSTORE PUSH1 0x20 ADD SWAP2 POP POP PUSH1 0x40 MLOAD DUP1 SWAP2 SUB SWAP1 RETURN JUMPDEST DUP1 PUSH1 0x0 PUSH1 0x0 POP DUP2 SWAP1 SSTORE POP JUMPDEST POP JUMP JUMPDEST PUSH1 0x0 PUSH1 0x0 PUSH1 0x0 POP SLOAD SWAP1 POP PUSH1 0x9E JUMP JUMPDEST SWAP1 JUMP ",
	"sourceMap": "24:189:0:-;;;;;;;;;",
	"linkReferences": {}
}
```

其中，opcodes字段便是合约代码编译后的操作码集合。

以PUSH1 0x60为例，可以在jump_table.go中找到对应的operation:

```
PUSH1: {
			execute:       makePush(1, 1),
			gasCost:       gasPush,
			validateStack: makeStackFunc(0, 1),
			valid:         true,
		}
```

此时EVM就会去执行makePush函数，同时通过gasPush计算该操作需要的gas费用。EVM内部通过pop不断进行出栈操作来处理整个操作码集，当栈为空的时候表示整个合约代码执行完毕得到最后的执行结果。

至此，有关EVM的源码研读就告一段落了。









