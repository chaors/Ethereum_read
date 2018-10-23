# 0x02 geth基本功能

要想研读以太坊源码，首先必须了解这些代码实现了哪些功能。这一篇我们通过联盟链的方式以两条链的交互来了解下以太坊核心的功能。

# 准备工作

### 创世区块配置文件

在之前的文章我们了解过创世区块的源码，知道其结构，也知道创世区块的产生需要一个叫做genesis.json的配置文件。该配置文件内容对应创世区块数据结构，如下：

```
{ 
    "config": {
          "chainId": 15,
          "homesteadBlock": 0,
          "eip155Block": 0,
          "eip158Block": 0
      },
      "coinbase" : "0x0000000000000000000000000000000000000000",
      "difficulty" : "0x40000",
      "extraData" : "",
      "gasLimit" : "0xffffffff",
      "nonce" : "0x0000000000000042",
      "mixhash" : "0x0000000000000000000000000000000000000000000000000000000000000000",
      "parentHash" : "0x0000000000000000000000000000000000000000000000000000000000000000",
      "timestamp" : "0x00",
      "alloc": {}
} 
```

### 联盟链环境构造

在自己的目录下新建两个目录aChain，bChain分别作为两条链的目录。并在各自的目录创建一个如上的genesis.json的配置文件。

接下来，我们需要利用以太坊的geth工具来初始化区块链。geth工具在之前搭建以太坊本地环境的文章中已经搭建好。

打开终端进入a链对应的目录，执行初始化命令：

```
// 根据配置文件初始化区块链
geth --datadir ./data-init1/ init genesis.json
```

![genesis初始化](https://upload-images.jianshu.io/upload_images/830585-c0b84ea2cd9877ae.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

执行成功后，我们发现在a链目录下多了一个存储节点数据的目录data-init1，进入查看。

![初始化目录](https://upload-images.jianshu.io/upload_images/830585-8e9ae599dae50e7e.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

同理，同样的方法初始化bChain。

# 联盟链构造

### 启动控制台

初始化操作执行成功后需要启动geth控制台，使用下列命令：

```
// networkid 指定网络id，1-4为系统内置，不建议使用
// nodiscover 表示不去主动发现peers节点
geth --datadir ./data-init1/ --networkid 88 --nodiscover console
```

终端出现下列界面，表示启动成功：

![启动geth成功](https://upload-images.jianshu.io/upload_images/830585-978f1c989eb2e481.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)


接着继续在bChain中启动geth，这时我们发现使用上面的命令会启动失败。这是为什么呢？

![错误提示](https://upload-images.jianshu.io/upload_images/830585-e8c111c7ef0e8668.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

根据错误提示我们知道，端口30303已经被绑定。因此，我们推测可能是aChain的geth占用了30303端口，其实从上一个图的椭圆标注中即可就看出geth端口号，但是我们在执行命令时并没有指定端口，所以得知不指定时默认端口30303.

这时，我们就需要手动指定端口号来启动bChain的geth控制台。

```
// port 指定geth端口号，默认30303
geth --datadir ./data-init2/ --port 30304 --networkid 88 --nodiscover console
```

这时发现，端口为30304的geth控制台启动成功。

![启动成功](https://upload-images.jianshu.io/upload_images/830585-1f1a85c1ae1d438f.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

### Coinbase账户

启动成功后，我们就可以给区块链添加一个账户。添加账户的命令在之前搭建以太坊环境时已经介绍过，这里回忆一下：

```
// 添加账户，密码为“chaors”
personal.newAccount("chaors")

// 列出当前所有账户
personal.listAccounts

// coinbase账户
eth.coinbase

// 钱包信息
personal.listWallets
```

为aChain创建一个密码为“chaors”的账户。

![创建账户](https://upload-images.jianshu.io/upload_images/830585-90d4f102479672c8.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

这时，我们发现原来初始化生成的秘钥目录下多了两个文件，因为我们创造了两个用户。

![秘钥文件](https://upload-images.jianshu.io/upload_images/830585-b7f9b93c5104951a.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

### 联盟链互通

现在已经存在两个相互独立的链，我们要做的就是将两个链互通。用到的命令有：

```
// 查看节点peers
admin.peers

// 查看节点信息
admin.nodeInfo.enode

// 添加节点参数为admin.nodeInfo.enode查询到信息内容
admin.addPeer("enode://e4b51e8bf54c82660e3123ff1d996cb4d9234bc1e8312b5144cc6e2d3538b33e8f8f438dad2f08cd968a408e31f5781535eaf1f1e5944e9af7c962ddd05a9594@[::]:30306?d
  iscport=0")
```

具体步骤如下：

![联盟链互通](https://upload-images.jianshu.io/upload_images/830585-8f0c786084ed35c5.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

添加成功后，再次利用admin.peers查看来验证是否成功。

![查看peers](https://upload-images.jianshu.io/upload_images/830585-5cb762a81cd74218.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

# 挖矿

已经有了链和账户的概念了，接下来就可以在账户上搞事情了。查询账户余额发现为零，这个时候我们就需要通过挖矿来生成奖励，使得账户余额不为零。

用到的命令有：

```
// 查询余额 参数为账户地址，这里查询的是矿工地址
eth.getBalance(eth.coinbase)

// 挖矿开始
miner.start()

// 挖矿结束
miner.stop()
```

在aChain执行挖矿,当aChain挖到区块时，我们发现bChain控制台在不断地打印信息。

![挖矿](https://upload-images.jianshu.io/upload_images/830585-cdffb11ef9a3cef7.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

由此可见，aChain在挖矿的同时，bChain在同步数据。

接下来，停止挖矿。分别在两个控制台查看矿工账户余额发现一致，说明挖矿的过程也同步成功。

![查询余额](https://upload-images.jianshu.io/upload_images/830585-efb5dca7b435e87b.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

# 交易

### 转账

账户有了余额之后就可以搞大事情了，可以在两个账户之间进行转账。用到的命令有：

```
// 解锁账户
personal.unlockAccount(eth.accounts[0])

// 转账
eth.sendTransaction({from: eth.coinbase, to: '0x02b7344004c45465796f779b7b95d7912
  c2ef572', value: 100000000})
```

首先在aChain中解锁账户并给bChain账户发生转账，发现bChain账户余额仍然为零。这是为什么呢？

![发起转账](https://upload-images.jianshu.io/upload_images/830585-d80311180868ea6d.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

因为，我们虽然发生了交易，但是并没有进行挖矿打包。aChain挖矿后进行查询发现bChain账户到账。

![打包交易](https://upload-images.jianshu.io/upload_images/830585-485eee789667b955.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

# 知其所以然？

现在基本了解了geth的功能，可以算是知其然了。下一步需要做的就是深入到源码去，在了解大概功能的基础上知其所以然。


.
.
.
.
>###互联网颠覆世界，区块链颠覆互联网!

>###### --------------------------------------------------20180905 23:28

















