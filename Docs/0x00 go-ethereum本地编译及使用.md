# 0x00 go-ethereum本地编译及使用
# 前言
 
比特币是区块链技术应用最成功的一个项目，也被业界公认为区块链1.0技术。随着区块链技术的普及和发展，出现了以太坊智能合约。

以太坊是一个建立在区块链技术之上的去中心化应用平台。我们可以在这个平台上建立使用区块链技术的去中心化应用。

可以这样理解，以太坊就好比一个开发平台(例如运行安卓程序的Android系统)，基于区块链技术的去中心化应用就好比Android系统上运行的Android应用。

这篇文章开始，将在本地编译以太坊源码。并且，初步地学习基于命令行的以太坊客户端基本使用。

# 本地编译

### 项目初窥

打开Ethereum的github地址：

Ethereum客户端有两种实现方式：go语言和c++。我们这里选择最常使用的go原因客户端。

![Ethereum项目](https://upload-images.jianshu.io/upload_images/830585-7b1d4af1b81f0c74.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

### 安装准备
- 1.go开发环境
- 2.Homebrew
- 3.Xcode环境

### 安装geth(go-ethereum)

```
//安装geth
brew tap ethereum/ethereum
brew install ethereum

cd ...
//远程拉取go-ethereum项目
git clone https://github.com/ethereum/go-ethereum
cd .../go-ethereum
//make
make geth
```
等待一会，ethereum本地编译就完成了。接着就可以在命令行启动并使用ethereum客户端。

# geth使用方法

### 基本使用范例
```
//1.从测试网络启动一个以太坊网络节点
geth --datadir testNet --dev console 2>> test.log

//2.查看账户，系统会有一个默认的账户
eth.accounts

//3.查看余额,由于是测试网络  默认账户会有大量的余额
eth.getBalance(eth.accounts[0])

//4.创建新账户,用户密码为chaors  可以用2查看
personal.newAccount("chaors")

//5.给新用户转账 从一个地址转给另一个地址9个以太币
eth.sendTransaction({from:'0x4ca5da2d66d9bf9074bd2fd097f468d92cd15d17',to:'0x67588df863e337e78b290cb77809197de1b2fc38',value:web3.toWei(9,"ether")})
```

![基本使用示例](https://upload-images.jianshu.io/upload_images/830585-3b1acf8042289bd0.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

### geth常用API
启动geth后我们在进入js控制台时候，会有一个提示，最下方列出了geth所有可以使用的模块：
```
...
modules: admin:1.0 clique:1.0 debug:1.0 eth:1.0 miner:1.0 net:1.0 personal:1.0 
rpc:1.0 shh:1.0 txpool:1.0 web3:1.0

```

```
eth：包含一些跟操作区块链相关的方法
net：包含以下查看p2p网络状态的方法
admin：包含一些与管理节点相关的方法
miner：包含启动&停止挖矿的一些方法
personal：主要包含一些管理账户的方法
txpool：包含一些查看交易内存池的方法
web3：包含了以上对象，还包含一些单位换算的方法
```

在js控制台输入任何一个模块名，都会列出该模块下所有的属性和函数。这样我们在使用geth的时候可以将模块名当做一个粗略的API说明，详细参阅[官方文档](https://web3js.readthedocs.io/en/1.0/index.html)。

使用模块粗略查询API ：

![eth模块](https://upload-images.jianshu.io/upload_images/830585-7abb7c9b89bab559.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

![personal模块](https://upload-images.jianshu.io/upload_images/830585-171ffe3c7349f039.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)



### geth命令详解

```
//命令格式
geth [选项] 命令 [命令选项] [参数…]

//主要命令
account           管理账户
attach            启动交互式JavaScript环境（连接到节点）
bug               上报bug Issues
console           启动交互式JavaScript环境
copydb            从文件夹创建本地链
dump              Dump（分析）一个特定的块存储
dumpconfig        显示配置值
export            导出区块链到文件
import            导入一个区块链文件
init              启动并初始化一个新的创世纪块
js                执行指定的JavaScript文件(多个)
license           显示许可信息
makecache         生成ethash验证缓存(用于测试)
makedag           生成ethash 挖矿DAG(用于测试)
monitor           监控和可视化节点指标
removedb          删除区块链和状态数据库
version           打印版本号
wallet            管理Ethereum预售钱包
help,h            显示一个命令或帮助一个命令列表

//ETHEREUM选项
--config value          TOML 配置文件
–-datadir “xxx”         数据库和keystore密钥的数据目录
–-keystore              keystore存放目录(默认在datadir内)
--nousb                 禁用监控和管理USB硬件钱包
–-networkid value       网络标识符(整型, 1=Frontier, 2=Morden (弃用), 3=Ropsten, 4=Rinkeby) (默认: 1)
–-testnet               Ropsten网络:预先配置的POW(proof-of-work)测试网络
--rinkeby               Rinkeby网络: 预先配置的POA(proof-of-authority)测试网络
--syncmode "fast"       同步模式 ("fast", "full", or "light")
--ethstats value        上报ethstats service  URL (nodename:secret@host:port)
--identity value        自定义节点名
--lightserv value       允许LES请求时间最大百分比(0 – 90)(默认值:0) 
--lightpeers value      最大LES client peers数量(默认值:20)
--lightkdf              在KDF强度消费时降低key-derivation RAM&CPU使用

//开发者选项
--dev                   使用POA共识网络，默认预分配一个开发者账户并且会自动开启挖矿。
--dev.period value      开发者模式下挖矿周期 (0 = 仅在交易时) (默认: 0)

//ETHASH选项
--ethash.cachedir               ethash验证缓存目录(默认 = datadir目录内)
--ethash.cachesinmem value      在内存保存的最近的ethash缓存个数  (每个缓存16MB ) (默认: 2)
--ethash.cachesondisk value     在磁盘保存的最近的ethash缓存个数 (每个缓存16MB) (默认: 3)
--ethash.dagdir ""              存ethash DAGs目录 (默认 = 用户hom目录)
--ethash.dagsinmem value        在内存保存的最近的ethash DAGs 个数 (每个1GB以上) (默认: 1)
--ethash.dagsondisk value       在磁盘保存的最近的ethash DAGs 个数 (每个1GB以上) (默认: 2)

//交易池选项
--txpool.nolocals            为本地提交交易禁用价格豁免
--txpool.journal value       本地交易的磁盘日志：用于节点重启 (默认: "transactions.rlp")
--txpool.rejournal value     重新生成本地交易日志的时间间隔 (默认: 1小时)
--txpool.pricelimit value    加入交易池的最小的gas价格限制(默认: 1)
--txpool.pricebump value     价格波动百分比（相对之前已有交易） (默认: 10)
--txpool.accountslots value  每个帐户保证可执行的最少交易槽数量  (默认: 16)
--txpool.globalslots value   所有帐户可执行的最大交易槽数量 (默认: 4096)
--txpool.accountqueue value  每个帐户允许的最多非可执行交易槽数量 (默认: 64)
--txpool.globalqueue value   所有帐户非可执行交易最大槽数量  (默认: 1024)
--txpool.lifetime value      非可执行交易最大入队时间(默认: 3小时)

//账户选项
––unlock value              需解锁账户用逗号分隔
––password value            用于非交互式密码输入的密码文件


//API和控制台选项
––rpc                       启用HTTP-RPC服务器
––rpcaddr value             HTTP-RPC服务器接口地址(默认值:“localhost”)
––rpcport value             HTTP-RPC服务器监听端口(默认值:8545)
––rpcapi value              基于HTTP-RPC接口提供的API
––ws                        启用WS-RPC服务器
––wsaddr value              WS-RPC服务器监听接口地址(默认值:“localhost”)
––wsport value              WS-RPC服务器监听端口(默认值:8546)
––wsapi  value              基于WS-RPC的接口提供的API
––wsorigins value           websockets请求允许的源
––ipcdisable                禁用IPC-RPC服务器
––ipcpath                   包含在datadir里的IPC socket/pipe文件名(转义过的显式路径)
––rpccorsdomain value       允许跨域请求的域名列表(逗号分隔)(浏览器强制)
––jspath loadScript         JavaScript加载脚本的根路径(默认值:“.”)
––exec value                执行JavaScript语句(只能结合console/attach使用)
––preload value             预加载到控制台的JavaScript文件列表(逗号分隔)

//网络选项
––bootnodes value           用于P2P发现引导的enode urls(逗号分隔)(对于light servers用v4+v5代替)
--bootnodesv4 value         用于P2P v4发现引导的enode urls(逗号分隔) (light server, 全节点)
--bootnodesv5 value         用于P2P v5发现引导的enode urls(逗号分隔) (light server, 轻节点)
-–port value                网卡监听端口(默认值:30303)
-–maxpeers value            最大的网络节点数量(如果设置为0，网络将被禁用)(默认值:25)
-–maxpendpeers value        最大尝试连接的数量(如果设置为0，则将使用默认值)(默认值:0)
-–nat value                 NAT端口映射机制 (any|none|upnp|pmp|extip:<IP>) (默认: “any”)
-–nodiscover                禁用节点发现机制(手动添加节点)
-–v5disc                    启用实验性的RLPx V5(Topic发现)机制
-–nodekey value             P2P节点密钥文件
-–nodekeyhex value         十六进制的P2P节点密钥(用于测试)

//矿工选项
––mine                  打开挖矿
––minerthreads value    挖矿使用的CPU线程数量(默认值:8)
––etherbase value       挖矿奖励地址(默认=第一个创建的帐户)(默认值:“0”)
––targetgaslimit value  目标gas限制：设置最低gas限制（低于这个不会被挖？） (默认值:“4712388”)
––gasprice value        挖矿接受交易的最低gas价格
––extradata value       矿工设置的额外块数据(默认=client version)

//GAS选项
--gpoblocks value       用于检查gas价格的最近块的个数  (默认: 10)
--gpopercentile value   建议gas价参考最近交易的gas价的百分位数，(默认: 50)

//调试选项
––metrics                   启用metrics收集和报告
––fakepow                   禁用proof-of-work验证
––verbosity value           日志详细度:0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail (default: 3)
––vmodule value             每个模块详细度:以 <pattern>=<level>的逗号分隔列表 (比如 eth/*=6,p2p=5)
––backtrace value           请求特定日志记录堆栈跟踪 (比如 “block.go:271”)
--debug                     突出显示调用位置日志(文件名及行号)
––pprof                     启用pprof HTTP服务器
––pprofaddr value           pprof HTTP服务器监听接口(默认值:127.0.0.1)
––pprofport value           pprof HTTP服务器监听端口(默认值:6060)
––memprofilerate value      按指定频率打开memory profiling    (默认:524288)
––blockprofilerate value    按指定频率打开block profiling    (默认值:0)
––cpuprofile value          将CPU profile写入指定文件
––trace value               将execution trace写入指定文件


//WHISPER选项
––shh                        启用Whisper
––shh.maxmessagesize value   可接受的最大的消息大小 (默认值: 1048576)
––shh.pow value              可接受的最小的POW (默认值: 0.2)

```

这样，Ethereum客户端的本地编译和基本使用就告一段落。下一篇我们开始[读源码来了解以太坊核心功能](https://www.jianshu.com/p/01db0c8023bc)。

### 参考资料

- [go-ethereum wiki](https://github.com/ethereum/go-ethereum/wiki/Installation-Instructions-for-Mac)

- [web3js](https://web3js.readthedocs.io/en/1.0/index.html)

- [go-ethereum Command Line Options](https://github.com/ethereum/go-ethereum/wiki/Command-Line-Options)





.
.
.
.
>### 互联网颠覆世界，区块链颠覆互联网!

>###### --------------------------------------------------20180424 22:01





