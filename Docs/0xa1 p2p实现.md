# 0xa1 p2p实现

首先，在之前的[go公链实战](https://www.jianshu.com/p/2b20f48ca301)中大概介绍了区块链网络的原理和实现，通信协议的实现参照的是Bitcoin的，这里以太坊的[通信协议](https://github.com/ethereum/wiki/wiki/Ethereum-Wire-Protocol)也大同小异。

以太坊devp2p最新动态和相关说明请点击[这里](https://github.com/ethereum/devp2p)。谈到devp2p就不得不讲[libp2p](https://libp2p.io/)，libp2p是一个开源的第三方p2p网络实现库，它汇集了各种传输和点对点协议，使开发人员可以轻松构建大型，强大的p2p网络。

# 废话少说撸代码

### p2p目录结构

![p2p目录结构](https://upload-images.jianshu.io/upload_images/830585-1c0ddbb6caa02b8c.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

### Server

我们就从服务器逻辑处理类来入手。

```
// Server manages all peer connections.
// 点对点连接的服务器管理者
type Server struct {
	// Config fields may not be modified while the server is running.
	// 服务器配置
	Config

	// Hooks for testing. These are useful because we can inhibit
	// the whole protocol stack.
	// 测试hook
	newTransport func(net.Conn) transport
	newPeerHook  func(*Peer)

	// 同步锁
	lock    sync.Mutex // protects running
	running bool

	ntab         discoverTable
	listener     net.Listener
	ourHandshake *protoHandshake
	lastLookup   time.Time
	DiscV5       *discv5.Network

	// These are for Peers, PeerCount (and nothing else).
	peerOp     chan peerOpFunc
	peerOpDone chan struct{}

	quit          chan struct{}
	addstatic     chan *discover.Node
	removestatic  chan *discover.Node
	posthandshake chan *conn
	addpeer       chan *conn
	delpeer       chan peerDrop
	loopWG        sync.WaitGroup // loop, listenLoop
	peerFeed      event.Feed
	log           log.Logger
}
...
// Config holds Server options.
type Config struct {
	// This field must be set to a valid secp256k1 private key.
	// 节点私钥
	PrivateKey *ecdsa.PrivateKey `toml:"-"`

	// MaxPeers is the maximum number of peers that can be
	// connected. It must be greater than zero.
	// 允许连接的最大节点数
	MaxPeers int

	// MaxPendingPeers is the maximum number of peers that can be pending in the
	// handshake phase, counted separately for inbound and outbound connections.
	// Zero defaults to preset values.
	// 握手阶段可以建立的最大连接数
	MaxPendingPeers int `toml:",omitempty"`

	// DialRatio controls the ratio of inbound to dialed connections.
	// Example: a DialRatio of 2 allows 1/2 of connections to be dialed.
	// Setting DialRatio to zero defaults it to 3.
	// 控制拨号比例 eg:DialRatio = 2 允许拨打1/2连接
	DialRatio int `toml:",omitempty"`

	// NoDiscovery can be used to disable the peer discovery mechanism.
	// Disabling is useful for protocol debugging (manual topology).
	// 禁用发现
	NoDiscovery bool

	// DiscoveryV5 specifies whether the the new topic-discovery based V5 discovery
	// protocol should be started or not.
	// 是否启用新的发现协议
	DiscoveryV5 bool `toml:",omitempty"`

	// Name sets the node name of this server.
	// Use common.MakeName to create a name that follows existing conventions.
	// 节点名称
	Name string `toml:"-"`

	// BootstrapNodes are used to establish connectivity
	// with the rest of the network.
	// 用于和网络其余部分建立连接
	BootstrapNodes []*discover.Node

	// BootstrapNodesV5 are used to establish connectivity
	// with the rest of the network using the V5 discovery
	// protocol.
	// 新的发现协议使用
	BootstrapNodesV5 []*discv5.Node `toml:",omitempty"`

	// Static nodes are used as pre-configured connections which are always
	// maintained and re-connected on disconnects.
	// 静态节点
	StaticNodes []*discover.Node

	// Trusted nodes are used as pre-configured connections which are always
	// allowed to connect, even above the peer limit.
	// 可信节点，始终允许连接
	TrustedNodes []*discover.Node

	// Connectivity can be restricted to certain IP networks.
	// If this option is set to a non-nil value, only hosts which match one of the
	// IP networks contained in the list are considered.
	// 限制连接设置
	// NetRestrict != 0 则只考虑与列表中包含的IP进行连接
	NetRestrict *netutil.Netlist `toml:",omitempty"`

	// NodeDatabase is the path to the database containing the previously seen
	// live nodes in the network.
	// 节点数据库路径
	NodeDatabase string `toml:",omitempty"`

	// Protocols should contain the protocols supported
	// by the server. Matching protocols are launched for
	// each peer.
	// 协议集
	Protocols []Protocol `toml:"-"`

	// If ListenAddr is set to a non-nil address, the server
	// will listen for incoming connections.
	//
	// If the port is zero, the operating system will pick a port. The
	// ListenAddr field will be updated with the actual address when
	// the server is started.
	// 监听地址
	// 如果ListenAddr设置为非零地址，则服务器将侦听传入连接
	// ListenAddr = 0 则系统选择一个端口
	ListenAddr string

	// If set to a non-nil value, the given NAT port mapper
	// is used to make the listening port available to the
	// Internet.
	// NAT端口
	NAT nat.Interface `toml:",omitempty"`

	// If Dialer is set to a non-nil value, the given Dialer
	// is used to dial outbound peer connections.
	// 拨号方
	Dialer NodeDialer `toml:"-"`

	// If NoDial is true, the server will not dial any peers.
	// 不与任何节点发起连接的标志
	NoDial bool `toml:",omitempty"`

	// If EnableMsgEvents is set then the server will emit PeerEvents
	// whenever a message is sent to or received from a peer
	// EnableMsgEvents = yes 则只要发起对等连接或接收消息，服务器就会发出PeerEvent
	EnableMsgEvents bool

	// Logger is a custom logger to use with the p2p.Server.
	// 客户端日志
	Logger log.Logger `toml:",omitempty"`
}
...
const (

	// 默认的连接超时时间
	defaultDialTimeout = 15 * time.Second

	// Connectivity defaults.
	// 默认连接属性
	maxActiveDialTasks     = 16
	defaultMaxPendingPeers = 50
	defaultDialRatio       = 3

	// Maximum time allowed for reading a complete message.
	// This is effectively the amount of time a connection can be idle.
	// 读取一个完整消息需要的最长时间(连接空闲时间)
	frameReadTimeout = 30 * time.Second

	// Maximum amount of time allowed for writing a complete message.
	// 构造一个完整消息徐亚的最长时间
	frameWriteTimeout = 20 * time.Second
)
```

然后看启动p2p服务的代码。

```
// Start starts running the server.
// Servers can not be re-used after stopping.
// 启动p2p
func (srv *Server) Start() (err error) {

	// 避免重复多次启动
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if srv.running {
		return errors.New("server already running")
	}
	srv.running = true
	srv.log = srv.Config.Logger
	if srv.log == nil {
		srv.log = log.New()
	}
	srv.log.Info("Starting P2P networking")

	// static fields
	if srv.PrivateKey == nil {
		return fmt.Errorf("Server.PrivateKey must be set to a non-nil key")
	}
	if srv.newTransport == nil {

		// 加密链路使用RLPX协议
		srv.newTransport = newRLPX
	}
	if srv.Dialer == nil {
		// 使用TCPDialer
		srv.Dialer = TCPDialer{&net.Dialer{Timeout: defaultDialTimeout}}
	}

	// 各种channel通道
	srv.quit = make(chan struct{})
	srv.addpeer = make(chan *conn)
	srv.delpeer = make(chan peerDrop)
	srv.posthandshake = make(chan *conn)
	srv.addstatic = make(chan *discover.Node)
	srv.removestatic = make(chan *discover.Node)
	srv.peerOp = make(chan peerOpFunc)
	srv.peerOpDone = make(chan struct{})

	var (
		conn      *net.UDPConn
		sconn     *sharedUDPConn
		realaddr  *net.UDPAddr
		unhandled chan discover.ReadPacket
	)

	if !srv.NoDiscovery || srv.DiscoveryV5 {
		// 启动discover网络
		addr, err := net.ResolveUDPAddr("udp", srv.ListenAddr)
		if err != nil {
			return err
		}
		// 开启UDP监听
		conn, err = net.ListenUDP("udp", addr)
		if err != nil {
			return err
		}
		realaddr = conn.LocalAddr().(*net.UDPAddr)
		if srv.NAT != nil {
			if !realaddr.IP.IsLoopback() {
				go nat.Map(srv.NAT, srv.quit, "udp", realaddr.Port, realaddr.Port, "ethereum discovery")
			}
			// TODO: react to external IP changes over time.
			if ext, err := srv.NAT.ExternalIP(); err == nil {
				realaddr = &net.UDPAddr{IP: ext, Port: realaddr.Port}
			}
		}
	}

	if !srv.NoDiscovery && srv.DiscoveryV5 {
		// 新的节点发现协议
		unhandled = make(chan discover.ReadPacket, 100)
		sconn = &sharedUDPConn{conn, unhandled}
	}

	// node table
	if !srv.NoDiscovery {
		cfg := discover.Config{
			PrivateKey:   srv.PrivateKey,
			AnnounceAddr: realaddr,
			NodeDBPath:   srv.NodeDatabase,
			NetRestrict:  srv.NetRestrict,
			Bootnodes:    srv.BootstrapNodes,
			Unhandled:    unhandled,
		}
		ntab, err := discover.ListenUDP(conn, cfg)
		if err != nil {
			return err
		}
		srv.ntab = ntab
	}

	if srv.DiscoveryV5 {
		var (
			ntab *discv5.Network
			err  error
		)
		if sconn != nil {
			ntab, err = discv5.ListenUDP(srv.PrivateKey, sconn, realaddr, "", srv.NetRestrict) //srv.NodeDatabase)
		} else {
			ntab, err = discv5.ListenUDP(srv.PrivateKey, conn, realaddr, "", srv.NetRestrict) //srv.NodeDatabase)
		}
		if err != nil {
			return err
		}
		if err := ntab.SetFallbackNodes(srv.BootstrapNodesV5); err != nil {
			return err
		}
		srv.DiscV5 = ntab
	}

	dynPeers := srv.maxDialedConns()
	// 新建dialerstate用来处理与节点的连接
	dialer := newDialState(srv.StaticNodes, srv.BootstrapNodes, srv.ntab, dynPeers, srv.NetRestrict)

	// handshake
	// 握手协议
	srv.ourHandshake = &protoHandshake{Version: baseProtocolVersion, Name: srv.Name, ID: discover.PubkeyID(&srv.PrivateKey.PublicKey)}
	for _, p := range srv.Protocols {
		srv.ourHandshake.Caps = append(srv.ourHandshake.Caps, p.cap())
	}
	// listen/dial
	if srv.ListenAddr != "" {
		// 开始监听TCP端口
		if err := srv.startListening(); err != nil {
			return err
		}
	}
	if srv.NoDial && srv.ListenAddr == "" {
		srv.log.Warn("P2P server will be useless, neither dialing nor listening")
	}

	srv.loopWG.Add(1)
	// 开启协程来处理
	go srv.run(dialer)
	srv.running = true
	return nil
}
```

接着继续来看里面，有关启动tcp监听的函数srv.startListening().它会启动底层来监听socket，当收到请求Accept后会调用setupConn()函数来建立连接。

setupConn()中握手主要分两个两个阶段：加密握手和协议握手。

```
func (srv *Server) startListening() error {
	// Launch the TCP listener.
	listener, err := net.Listen("tcp", srv.ListenAddr)
	if err != nil {
		return err
	}
	laddr := listener.Addr().(*net.TCPAddr)
	srv.ListenAddr = laddr.String()
	srv.listener = listener
	srv.loopWG.Add(1)
	go srv.listenLoop()
	// Map the TCP listening port if NAT is configured.
	// 匹配TCP端口
	if !laddr.IP.IsLoopback() && srv.NAT != nil {
		srv.loopWG.Add(1)
		go func() {
			nat.Map(srv.NAT, srv.quit, "tcp", laddr.Port, laddr.Port, "ethereum p2p")
			srv.loopWG.Done()
		}()
	}
	return nil
}
...
// listenLoop runs in its own goroutine and accepts
// inbound connections.
// 循环监听的一个协程
func (srv *Server) listenLoop() {
	defer srv.loopWG.Done()
	srv.log.Info("RLPx listener up", "self", srv.makeSelf(srv.listener, srv.ntab))

	// 连接处理数
	tokens := defaultMaxPendingPeers
	if srv.MaxPendingPeers > 0 {
		tokens = srv.MaxPendingPeers
	}
	slots := make(chan struct{}, tokens)
	for i := 0; i < tokens; i++ {
		slots <- struct{}{}
	}

	for {
		// Wait for a handshake slot before accepting.
		// 等待握手信号槽
		<-slots

		var (
			fd  net.Conn
			err error
		)
		for {
			fd, err = srv.listener.Accept()
			if tempErr, ok := err.(tempError); ok && tempErr.Temporary() {
				srv.log.Debug("Temporary read error", "err", err)
				continue
			} else if err != nil {
				srv.log.Debug("Read error", "err", err)
				return
			}
			break
		}

		// Reject connections that do not match NetRestrict.
		// 拒绝不在白名单NetRestrict里的连接
		if srv.NetRestrict != nil {
			if tcp, ok := fd.RemoteAddr().(*net.TCPAddr); ok && !srv.NetRestrict.Contains(tcp.IP) {
				srv.log.Debug("Rejected conn (not whitelisted in NetRestrict)", "addr", fd.RemoteAddr())
				fd.Close()
				slots <- struct{}{}
				continue
			}
		}

		fd = newMeteredConn(fd, true)
		srv.log.Trace("Accepted connection", "addr", fd.RemoteAddr())
		go func() {
			// 执行连接
			srv.SetupConn(fd, inboundConn, nil)
			slots <- struct{}{}
		}()
	}
}
...
// SetupConn runs the handshakes and attempts to add the connection
// as a peer. It returns when the connection has been added as a peer
// or the handshakes have failed.
// 执行握手并尝试将连接方作为一个peer
func (srv *Server) SetupConn(fd net.Conn, flags connFlag, dialDest *discover.Node) error {
	self := srv.Self()
	if self == nil {
		return errors.New("shutdown")
	}
	// 创建conn对象
	c := &conn{fd: fd, transport: srv.newTransport(fd), flags: flags, cont: make(chan error)}
	err := srv.setupConn(c, flags, dialDest)
	if err != nil {
		c.close(err)
		srv.log.Trace("Setting up connection failed", "id", c.id, "err", err)
	}
	return err
}

func (srv *Server) setupConn(c *conn, flags connFlag, dialDest *discover.Node) error {
	// Prevent leftover pending conns from entering the handshake.
	srv.lock.Lock()
	running := srv.running
	srv.lock.Unlock()
	if !running {
		return errServerStopped
	}
	// Run the encryption handshake.
	var err error
	// 握手协议的RLPX编码
	if c.id, err = c.doEncHandshake(srv.PrivateKey, dialDest); err != nil {
		srv.log.Trace("Failed RLPx handshake", "addr", c.fd.RemoteAddr(), "conn", c.flags, "err", err)
		return err
	}
	clog := srv.log.New("id", c.id, "addr", c.fd.RemoteAddr(), "conn", c.flags)
	// For dialed connections, check that the remote public key matches.
	// 连接握手的ID和对应的ID不匹配
	if dialDest != nil && c.id != dialDest.ID {
		clog.Trace("Dialed identity mismatch", "want", c, dialDest.ID)
		return DiscUnexpectedIdentity
	}
	// 将c发送给srv.posthandshake
	err = srv.checkpoint(c, srv.posthandshake)
	if err != nil {
		clog.Trace("Rejected peer before protocol handshake", "err", err)
		return err
	}
	// Run the protocol handshake
	// 运行握手协议
	phs, err := c.doProtoHandshake(srv.ourHandshake)
	if err != nil {
		clog.Trace("Failed proto handshake", "err", err)
		return err
	}
	if phs.ID != c.id {
		clog.Trace("Wrong devp2p handshake identity", "err", phs.ID)
		return DiscUnexpectedIdentity
	}
	c.caps, c.name = phs.Caps, phs.Name
	// 将c发送给addpeer队列
	err = srv.checkpoint(c, srv.addpeer)
	if err != nil {
		clog.Trace("Rejected peer", "err", err)
		return err
	}
	// If the checks completed successfully, runPeer has now been
	// launched by run.
	clog.Trace("connection set up", "inbound", dialDest == nil)
	return nil
}
```
在server.start()还有个run()函数，它主要用来处理主动连接外部节点的流程和已经连接的checkpoint逻辑。

```
func (srv *Server) run(dialstate dialer) {
	defer srv.loopWG.Done()
	var (
		peers        = make(map[discover.NodeID]*Peer)
		inboundCount = 0
		trusted      = make(map[discover.NodeID]bool, len(srv.TrustedNodes))
		taskdone     = make(chan task, maxActiveDialTasks)
		runningTasks []task
		queuedTasks  []task // tasks that can't run yet
	)
	// Put trusted nodes into a map to speed up checks.
	// Trusted peers are loaded on startup and cannot be
	// modified while the server is running.
	// 将信任节点加入map以快速检错
	for _, n := range srv.TrustedNodes {
		trusted[n.ID] = true
	}

	// removes t from runningTasks
	// delTask函数用来从runningTasks删除一个task
	delTask := func(t task) {
		for i := range runningTasks {
			if runningTasks[i] == t {
				runningTasks = append(runningTasks[:i], runningTasks[i+1:]...)
				break
			}
		}
	}
	// starts until max number of active tasks is satisfied
	// 启动任务，直到任务数达到最大
	startTasks := func(ts []task) (rest []task) {
		i := 0
		for ; len(runningTasks) < maxActiveDialTasks && i < len(ts); i++ {
			t := ts[i]
			srv.log.Trace("New dial task", "task", t)
			go func() { t.Do(srv); taskdone <- t }()
			runningTasks = append(runningTasks, t)
		}
		return ts[i:]
	}
	scheduleTasks := func() {
		// Start from queue first.
		// 启动queuedTasks中的第一个
		queuedTasks = append(queuedTasks[:0], startTasks(queuedTasks)...)
		// Query dialer for new tasks and start as many as possible now.
		// 调用newTasks来生成任务，并尝试用startTasks启动
		// 把暂时无法启动的放入queuedTasks队列
		if len(runningTasks) < maxActiveDialTasks {
			nt := dialstate.newTasks(len(runningTasks)+len(queuedTasks), peers, time.Now())
			queuedTasks = append(queuedTasks, startTasks(nt)...)
		}
	}

running:
	for {
		scheduleTasks()

		select {
		case <-srv.quit:
			// The server was stopped. Run the cleanup logic.
			// 服务器停止
			break running
		case n := <-srv.addstatic:
			// This channel is used by AddPeer to add to the
			// ephemeral static peer list. Add it to the dialer,
			// it will keep the node connected.
			// 添加静态节点
			srv.log.Trace("Adding static node", "node", n)
			dialstate.addStatic(n)
		case n := <-srv.removestatic:
			// This channel is used by RemovePeer to send a
			// disconnect request to a peer and begin the
			// stop keeping the node connected
			// 移除静态节点
			srv.log.Trace("Removing static node", "node", n)
			dialstate.removeStatic(n)
			if p, ok := peers[n.ID]; ok {
				p.Disconnect(DiscRequested)
			}
		case op := <-srv.peerOp:
			// This channel is used by Peers and PeerCount.
			op(peers)
			srv.peerOpDone <- struct{}{}
		case t := <-taskdone:
			// A task got done. Tell dialstate about it so it
			// can update its state and remove it from the active
			// tasks list.
			srv.log.Trace("Dial task done", "task", t)
			dialstate.taskDone(t, time.Now())
			delTask(t)
		case c := <-srv.posthandshake:
			// A connection has passed the encryption handshake so
			// the remote identity is known (but hasn't been verified yet).
			// 之前调用checkpoint()会把连接发送到这
			if trusted[c.id] {
				// Ensure that the trusted flag is set before checking against MaxPeers.
				// 确保在检查MaxPeers之前设置了可信标志。
				c.flags |= trustedConn
			}
			// TODO: track in-progress inbound node IDs (pre-Peer) to avoid dialing them.
			select {
			case c.cont <- srv.encHandshakeChecks(peers, inboundCount, c):
			case <-srv.quit:
				break running
			}
		case c := <-srv.addpeer:
			// At this point the connection is past the protocol handshake.
			// Its capabilities are known and the remote identity is verified.
			// 已经通过握手协议
			err := srv.protoHandshakeChecks(peers, inboundCount, c)
			if err == nil {
				// The handshakes are done and it passed all checks.
				// 节点通过验证
				p := newPeer(c, srv.Protocols)
				// If message events are enabled, pass the peerFeed
				// to the peer
				// 如果启用了消息事件，请将peerFeed传递给peer
				if srv.EnableMsgEvents {
					p.events = &srv.peerFeed
				}
				name := truncateName(c.name)
				srv.log.Debug("Adding p2p peer", "name", name, "addr", c.fd.RemoteAddr(), "peers", len(peers)+1)
				// 添加p2p节点
				go srv.runPeer(p)
				peers[c.id] = p
				if p.Inbound() {
					inboundCount++
				}
			}
			// The dialer logic relies on the assumption that
			// dial tasks complete after the peer has been added or
			// discarded. Unblock the task last.
			select {
			case c.cont <- err:
			case <-srv.quit:
				break running
			}
		case pd := <-srv.delpeer:
			// A peer disconnected.
			// 删除peer
			d := common.PrettyDuration(mclock.Now() - pd.created)
			pd.log.Debug("Removing p2p peer", "duration", d, "peers", len(peers)-1, "req", pd.requested, "err", pd.err)
			delete(peers, pd.ID())
			if pd.Inbound() {
				inboundCount--
			}
		}
	}

	srv.log.Trace("P2P networking is spinning down")

	// Terminate discovery. If there is a running lookup it will terminate soon.
	// 停止发现节点
	if srv.ntab != nil {
		srv.ntab.Close()
	}
	if srv.DiscV5 != nil {
		srv.DiscV5.Close()
	}
	// Disconnect all peers.
	// 断开与所有peer的连接
	for _, p := range peers {
		p.Disconnect(DiscQuitting)
	}
	// Wait for peers to shut down. Pending connections and tasks are
	// not handled here and will terminate soon-ish because srv.quit
	// is closed.
	// 等待peer关闭
	for len(peers) > 0 {
		p := <-srv.delpeer
		p.log.Trace("<-delpeer (spindown)", "remainingTasks", len(runningTasks))
		delete(peers, p.ID())
	}
}
...
// runPeer runs in its own goroutine for each peer.
// it waits until the Peer logic returns and removes
// the peer.
func (srv *Server) runPeer(p *Peer) {
	if srv.newPeerHook != nil {
		srv.newPeerHook(p)
	}

	// broadcast peer add
	// 广播节点添加
	srv.peerFeed.Send(&PeerEvent{
		Type: PeerEventTypeAdd,
		Peer: p.ID(),
	})

	// run the protocol
	// 运行协议
	remoteRequested, err := p.run()

	// broadcast peer drop
	srv.peerFeed.Send(&PeerEvent{
		Type:  PeerEventTypeDrop,
		Peer:  p.ID(),
		Error: err.Error(),
	})

	// Note: run waits for existing peers to be sent on srv.delpeer
	// before returning, so this send should not select on srv.quit.
	srv.delpeer <- peerDrop{p, err, remoteRequested}
}
```

总结一下，当server启动后，它会通过listenloop()来接收和处理外部请求，通过run来主动发起与外部其他节点的连接。

### Node

所以，接着就看看node的结构。

```
// Node represents a host on the network.
// The fields of Node may not be modified.
// 节点表示网络上的主机
type Node struct {
	IP       net.IP // len 4 for IPv4 or 16 for IPv6
	UDP, TCP uint16 // port numbers
	ID       NodeID // the node's public key

	// This is a cached copy of sha3(ID) which is used for node
	// distance calculations. This is part of Node in order to make it
	// possible to write tests that need a node at a certain distance.
	// In those tests, the content of sha will not actually correspond
	// with ID.
	sha common.Hash

	// Time when the node was added to the table.
	addedAt time.Time
}
```

连接中的远程节点peer的结构是：

```
// Peer represents a connected remote node.
// 连接的远程节点
type Peer struct {
	rw      *conn
	running map[string]*protoRW
	log     log.Logger
	created mclock.AbsTime

	wg       sync.WaitGroup
	protoErr chan error
	closed   chan struct{}
	disc     chan DiscReason

	// events receives message send / receive events if set
	events *event.Feed
}
```

在p2p网络中，每一个节点既是服务端又是客户端。devp2p主要有两个功能，一个是用来发现当前节点之外的其他节点，一个是实现两个节点间的信息通讯。

# 节点发现

### 准备知识

对于分布式系统，怎么去高效地发现节点是一个至关重要的问题。一般的分布式系统都是使用DHT(Distributed Hash Table，分布式哈希表)原理来发现和管理节点。其中应用最广的又当属Kademlia算法。

这里且通过我认为比较好的两篇博文来理解DHT原理和Kademlia算法。

- [聊聊分布式散列表（DHT）的原理](https://www.jianshu.com/p/ea322f256c24)

- [易懂分布式 | Kademlia算法](https://www.jianshu.com/p/f2c31e632f1d)

其中，《易懂分布式 | Kademlia算法》一文可以着重看看，它将是接下来看发现节点源码的基础，这也是我看过的关于Kademlia最通俗易懂的博文。

这里也将Kademlia算法的论文原文以及中文翻译版贴出:

- [Kademlia: A peerto -peer information system based on the XOR metric](https://pdos.csail.mit.edu/~petar/papers/maymounkov-kademlia-lncs.pdf)

- [Kademlia ：一种基于 XOR 度量的 P2P 信息系统](https://blog.csdn.net/hoping/article/details/5307320)

### table(k-bucket in Kademlia)

这里的table结构就相当于Kademlia算法中的k-bucket(k桶)，用来存储一个节点附近的某些节点路由。

```
const (
	// 并发因子
	alpha           = 3  // Kademlia concurrency factor
	// k桶容量
	bucketSize      = 16 // Kademlia bucket size
	// 每个k桶更换清单的大小
	maxReplacements = 10 // Size of per-bucket replacement list

	// We keep buckets for the upper 1/15 of distances because
	// it's very unlikely we'll ever encounter a node that's closer.
	//
	hashBits          = len(common.Hash{}) * 8
	nBuckets          = hashBits / 15       // Number of buckets
	// 最近k桶的距离
	bucketMinDistance = hashBits - nBuckets // Log distance of closest bucket

	// IP address limits.
	bucketIPLimit, bucketSubnet = 2, 24 // at most 2 addresses from the same /24
	tableIPLimit, tableSubnet   = 10, 24

	// 节点丢弃的限制，超过这些限制的节点将被丢弃
	maxFindnodeFailures = 5 // Nodes exceeding this limit are dropped
	refreshInterval     = 30 * time.Minute
	revalidateInterval  = 10 * time.Second
	copyNodesInterval   = 30 * time.Second
	seedMinTableTime    = 5 * time.Minute
	seedCount           = 30
	seedMaxAge          = 5 * 24 * time.Hour
)

//k-bucket表
type Table struct {
	mutex   sync.Mutex        // protects buckets, bucket content, nursery, rand
	// k桶数组
	buckets [nBuckets]*bucket // index of known nodes by distance
	// 引导节点
	nursery []*Node           // bootstrap nodes
	// 随机源头
	rand    *mrand.Rand       // source of randomness, periodically reseeded
	ips     netutil.DistinctNetSet

	// 节点数据库
	db         *nodeDB // database of known nodes
	refreshReq chan chan struct{}
	initDone   chan struct{}
	closeReq   chan struct{}
	closed     chan struct{}

	nodeAddedHook func(*Node) // for testing

	net  transport
	self *Node // metadata of the local node
}
...
// bucket contains nodes, ordered by their last activity. the entry
// that was most recently active is the first element in entries.
// 按上一个活动排序的节点
// 最近活动的条目是条目中的第一个元素
type bucket struct {
	// 实时节点列表
	entries      []*Node // live entries, sorted by time of last contact
	// 候补节点，entries满了后之后的节点会存储到这
	replacements []*Node // recently seen nodes to be used if revalidation fails
	ips          netutil.DistinctNetSet
}
```

那么怎么初始化一个k桶表呢？

```
func newTable(t transport, ourID NodeID, ourAddr *net.UDPAddr, nodeDBPath string, bootnodes []*Node) (*Table, error) {
	// If no node database was given, use an in-memory one
	// 1.新建节点数据库
	db, err := newNodeDB(nodeDBPath, nodeDBVersion, ourID)
	if err != nil {
		return nil, err
	}

	// 2.构造k桶表
	tab := &Table{
		net:        t,
		db:         db,
		self:       NewNode(ourID, ourAddr.IP, uint16(ourAddr.Port), uint16(ourAddr.Port)),
		refreshReq: make(chan chan struct{}),
		initDone:   make(chan struct{}),
		closeReq:   make(chan struct{}),
		closed:     make(chan struct{}),
		rand:       mrand.New(mrand.NewSource(0)),
		ips:        netutil.DistinctNetSet{Subnet: tableSubnet, Limit: tableIPLimit},
	}

	// 3.设置初始连接点
	if err := tab.setFallbackNodes(bootnodes); err != nil {
		return nil, err
	}

	// 4.填充k桶
	for i := range tab.buckets {
		tab.buckets[i] = &bucket{
			ips: netutil.DistinctNetSet{Subnet: bucketSubnet, Limit: bucketIPLimit},
		}
	}

	// 4.加载种子节点
	tab.seedRand()
	tab.loadSeedNodes()
	// Start the background expiration goroutine after loading seeds so that the search for
	// seed nodes also considers older nodes that would otherwise be removed by the
	// expiration.
	tab.db.ensureExpirer()
	
	// 5.开启协程循环更新
	go tab.loop()
	return tab, nil
}
...
// loop schedules refresh, revalidate runs and coordinates shutdown.
func (tab *Table) loop() {
	var (
		revalidate     = time.NewTimer(tab.nextRevalidateTime())
		refresh        = time.NewTicker(refreshInterval)
		copyNodes      = time.NewTicker(copyNodesInterval)
		revalidateDone = make(chan struct{})
		refreshDone    = make(chan struct{})           // where doRefresh reports completion
		waiting        = []chan struct{}{tab.initDone} // holds waiting callers while doRefresh runs
	)
	defer refresh.Stop()
	defer revalidate.Stop()
	defer copyNodes.Stop()

	// Start initial refresh.
	// 初始化刷新
	go tab.doRefresh(refreshDone)

loop:
	for {
		select {
		case <-refresh.C:
			tab.seedRand()
			if refreshDone == nil {
				refreshDone = make(chan struct{})
				go tab.doRefresh(refreshDone)
			}
		case req := <-tab.refreshReq:
			// 将刷新请求加入通道数组
			waiting = append(waiting, req)
			if refreshDone == nil {
				refreshDone = make(chan struct{})
				go tab.doRefresh(refreshDone)
			}
		case <-refreshDone:
			// 刷新完毕，关闭通道
			for _, ch := range waiting {
				close(ch)
			}
			waiting, refreshDone = nil, nil
		case <-revalidate.C:
			// 重新验证
			go tab.doRevalidate(revalidateDone)
		case <-revalidateDone:
			revalidate.Reset(tab.nextRevalidateTime())
		case <-copyNodes.C:
			// 拷贝活跃节点
			go tab.copyLiveNodes()
		case <-tab.closeReq:
			break loop
		}
	}

	if tab.net != nil {
		tab.net.close()
	}
	if refreshDone != nil {
		<-refreshDone
	}
	for _, ch := range waiting {
		close(ch)
	}
	tab.db.close()
	close(tab.closed)
}
```

接着继续看看loop里的几个函数。

```
// Lookup performs a network search for nodes close
// to the given target. It approaches the target by querying
// nodes that are closer to it on each iteration.
// The given target does not need to be an actual node
// identifier.
// 搜索当前节点的附近节点
func (tab *Table) Lookup(targetID NodeID) []*Node {
	return tab.lookup(targetID, true)
}

func (tab *Table) lookup(targetID NodeID, refreshIfEmpty bool) []*Node {
	var (
		target         = crypto.Keccak256Hash(targetID[:])
		asked          = make(map[NodeID]bool)
		seen           = make(map[NodeID]bool)
		reply          = make(chan []*Node, alpha)
		pendingQueries = 0
		result         *nodesByDistance
	)
	// don't query further if we hit ourself.
	// unlikely to happen often in practice.
	// 一般询问自己是没有意义的
	asked[tab.self.ID] = true

	for {
		tab.mutex.Lock()
		// generate initial result set
		// 获取k桶表中给定节点id的n个节点
		result = tab.closest(target, bucketSize)
		tab.mutex.Unlock()
		if len(result.entries) > 0 || !refreshIfEmpty {

			// k桶表不为空
			break
		}
		// The result set is empty, all nodes were dropped, refresh.
		// We actually wait for the refresh to complete here. The very
		// first query will hit this case and run the bootstrapping
		// logic.

		// k桶表为空
		<-tab.refresh()
		refreshIfEmpty = false
	}

	for {
		// ask the alpha closest nodes that we haven't asked yet
		// 询问尚未询问的最接近的节点
		for i := 0; i < len(result.entries) && pendingQueries < alpha; i++ {
			n := result.entries[i]
			// 未被查询的节点
			if !asked[n.ID] {
				asked[n.ID] = true
				pendingQueries++
				// 查找节点
				go tab.findnode(n, targetID, reply)
			}
		}
		if pendingQueries == 0 {
			// we have asked all closest nodes, stop the search
			// 所有最近节点都询问过了
			break
		}
		// wait for the next reply
		for _, n := range <-reply {
			if n != nil && !seen[n.ID] {
				seen[n.ID] = true
				result.push(n, bucketSize)
			}
		}
		pendingQueries--
	}
	return result.entries
}
...
func (tab *Table) findnode(n *Node, targetID NodeID, reply chan<- []*Node) {
	fails := tab.db.findFails(n.ID)
	r, err := tab.net.findnode(n.ID, n.addr(), targetID)
	if err != nil || len(r) == 0 {
		fails++
		tab.db.updateFindFails(n.ID, fails)
		log.Trace("Findnode failed", "id", n.ID, "failcount", fails, "err", err)
		if fails >= maxFindnodeFailures {
			log.Trace("Too many findnode failures, dropping", "id", n.ID, "failcount", fails)
			tab.delete(n)
		}
	} else if fails > 0 {
		tab.db.updateFindFails(n.ID, fails-1)
	}

	// Grab as many nodes as possible. Some of them might not be alive anymore, but we'll
	// just remove those again during revalidation.
	// 将所有节点加入k桶表，尽管有些已经离线(我们会在重新验证时删除这些节点)
	for _, n := range r {
		tab.add(n)
	}
	reply <- r
}
...
// nodesByDistance is a list of nodes, ordered by
// distance to target.
// 根据距离远近排序的邻居节点
type nodesByDistance struct {
	entries []*Node
	target  common.Hash
}

// push adds the given node to the list, keeping the total size below maxElems.
// 将节点添加到给定列表并维持大小小于maxElems
// 
func (h *nodesByDistance) push(n *Node, maxElems int) {
	
	// 根据节点到target的距离排序
	ix := sort.Search(len(h.entries), func(i int) bool {
		return distcmp(h.target, h.entries[i].sha, n.sha) > 0
	})
	
	// 大小未超出限制,将节点加入
	if len(h.entries) < maxElems {
		h.entries = append(h.entries, n)
	}
	if ix == len(h.entries) {
		// farther away than all nodes we already have.
		// if there was room for it, the node is now the last element.
	} else {
		// slide existing entries down to make room
		// this will overwrite the entry we just appended.
		copy(h.entries[ix+1:], h.entries[ix:])
		h.entries[ix] = n
	}
}
```

现在只看了刷新节点的方法doRefresh(),查找到的节点还需要进行二次验证来确保每一个节点都在线。

```
// doRevalidate checks that the last node in a random bucket is still live
// and replaces or deletes the node if it isn't.
// 检查最后一个节点是否在线，不在线就更换或删除
func (tab *Table) doRevalidate(done chan<- struct{}) {
	defer func() { done <- struct{}{} }()

	last, bi := tab.nodeToRevalidate()
	if last == nil {
		// No non-empty bucket found.
		return
	}

	// Ping the selected node and wait for a pong.
	// 发出ping指令，等待pong回复
	err := tab.net.ping(last.ID, last.addr())

	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	b := tab.buckets[bi]
	if err == nil {
		// The node responded, move it to the front.
		log.Trace("Revalidated node", "b", bi, "id", last.ID)
		b.bump(last)
		return
	}
	// No reply received, pick a replacement or delete the node if there aren't
	// any replacements.
	// 没有事收到ping的回复，更换或删除节点
	if r := tab.replace(b, last); r != nil {
		log.Trace("Replaced dead node", "b", bi, "id", last.ID, "ip", last.IP, "r", r.ID, "rip", r.IP)
	} else {
		log.Trace("Removed dead node", "b", bi, "id", last.ID, "ip", last.IP)
	}
}

// nodeToRevalidate returns the last node in a random, non-empty bucket.
// 获取随机非空k桶的最后一个节点
func (tab *Table) nodeToRevalidate() (n *Node, bi int) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	for _, bi = range tab.rand.Perm(len(tab.buckets)) {
		b := tab.buckets[bi]
		if len(b.entries) > 0 {
			last := b.entries[len(b.entries)-1]
			return last, bi
		}
	}
	return nil, 0
}
...
// bump moves the given node to the front of the bucket entry list
// if it is contained in that list.
// 将节点node移动到最前
func (b *bucket) bump(n *Node) bool {
	for i := range b.entries {
		if b.entries[i].ID == n.ID {
			// move it to the front
			copy(b.entries[1:], b.entries[:i])
			b.entries[0] = n
			return true
		}
	}
	return false
}
...
// replace removes n from the replacement list and replaces 'last' with it if it is the
// last entry in the bucket. If 'last' isn't the last entry, it has either been replaced
// with someone else or became active.
// 这里验证失败后会把前面buket结构里replacements里的节点候补到entries
func (tab *Table) replace(b *bucket, last *Node) *Node {
	if len(b.entries) == 0 || b.entries[len(b.entries)-1].ID != last.ID {
		// Entry has moved, don't replace it.
		return nil
	}
	// Still the last entry.
	if len(b.replacements) == 0 {
		tab.deleteInBucket(b, last)
		return nil
	}
	r := b.replacements[tab.rand.Intn(len(b.replacements))]
	b.replacements = deleteNode(b.replacements, r)
	b.entries[len(b.entries)-1] = r
	tab.removeIP(b, last.IP)
	return r
}
```

综上，table主要实现了用于p2p的Kademlia协议。主要通过loop函数来进行k桶表的构造，大概的流程是:

- 1.loadSeedNodes方法加载种子节点，通过这些节点来发现其他网络节点。

- 2.loop循环当收到刷新请求时，会调lookup方法去发现当前节点的种子节点

- 3.按照Kademlia核心思想，递归地调用findnode方法询问所有最近节点直到所有节点都被询问过

- 4.当收到一个节点replay时调用push方法将节点加入到nodesByDistance结构的数组，并按照各邻居节点到当前节点的顺序排序

- 5.然后还会通过doRevalidate方法发出ping指令对搜索到的bucket中的节点进行验证，以此来剔除不在线的但已添加到entries列表的节点

上面涉及到的ping()方法以及最终findnode方法都是通过transport接口实现的。不难发现udp这个类正好实现了这个接口，还有kademlia协议本身就是使用UDP来进行网络通信的。

```
// transport is implemented by the UDP transport.
// it is an interface so we can test without opening lots of UDP
// sockets and without generating a private key.
// 真正寻找节点的接口
type transport interface {
	ping(NodeID, *net.UDPAddr) error
	findnode(toid NodeID, addr *net.UDPAddr, target NodeID) ([]*Node, error)
	close()
}
```

### udp

进入udp.go，可以看到其中定义了4种数据报结构：

```
// RPC packet types
const (
	// ping包  用于询问节点是否在线
	pingPacket = iota + 1 // zero is 'reserved'
	//pong包 是对ping的回复
	pongPacket
	// 查找邻居节点
	findnodePacket
	// 返回另据节点  是对findnodePacket的回复
	neighborsPacket
)

// RPC request structures
type (
	ping struct {
		Version    uint
		From, To   rpcEndpoint
		Expiration uint64
		// Ignore additional fields (for forward compatibility).
		Rest []rlp.RawValue `rlp:"tail"`
	}

	// pong is the reply to ping.
	pong struct {
		// This field should mirror the UDP envelope address
		// of the ping packet, which provides a way to discover the
		// the external address (after NAT).
		To rpcEndpoint

		ReplyTok   []byte // This contains the hash of the ping packet.
		Expiration uint64 // Absolute timestamp at which the packet becomes invalid.
		// Ignore additional fields (for forward compatibility).
		Rest []rlp.RawValue `rlp:"tail"`
	}

	// findnode is a query for nodes close to the given target.
	findnode struct {
		Target     NodeID // doesn't need to be an actual public key
		Expiration uint64
		// Ignore additional fields (for forward compatibility).
		Rest []rlp.RawValue `rlp:"tail"`
	}

	// reply to findnode
	neighbors struct {
		Nodes      []rpcNode
		Expiration uint64
		// Ignore additional fields (for forward compatibility).
		Rest []rlp.RawValue `rlp:"tail"`
	}

	rpcNode struct {
		IP  net.IP // len 4 for IPv4 or 16 for IPv6
		UDP uint16 // for discovery protocol
		TCP uint16 // for RLPx protocol
		ID  NodeID
	}

	rpcEndpoint struct {
		IP  net.IP // len 4 for IPv4 or 16 for IPv6
		UDP uint16 // for discovery protocol
		TCP uint16 // for RLPx protocol
	}
)
```

接着来看一下udp的数据结构。

```
// conn接口实现了udp通讯必要的方法
type conn interface {
	ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error)
	WriteToUDP(b []byte, addr *net.UDPAddr) (n int, err error)
	Close() error
	LocalAddr() net.Addr
}

// udp implements the RPC protocol.
type udp struct {
	// conn接口实现了udp通讯必要的方法
	conn        conn
	netrestrict *netutil.Netlist
	// 节点私钥
	priv        *ecdsa.PrivateKey
	ourEndpoint rpcEndpoint

	// 待处理的回应，存储发出一个消息后应当得到的回应
	addpending chan *pending
	// 处理回应，收到回复消息后会遍历pending列表查找匹配的回应
	gotreply   chan reply

	closing chan struct{}
	nat     nat.Interface

	// 匿名对象，可以直接调用Table的方法
	*Table
}

// pending represents a pending reply.
//
// some implementations of the protocol wish to send more than one
// reply packet to findnode. in general, any neighbors packet cannot
// be matched up with a specific findnode packet.
//
// our implementation handles this by storing a callback function for
// each pending reply. incoming packets from a node are dispatched
// to all the callback functions for that node.
// 待处理的回复
//
type pending struct {
	// these fields must match in the reply.
	from  NodeID
	ptype byte

	// time when the request must complete
	// 请求消息的截止时间
	deadline time.Time

	// callback is called when a matching reply arrives. if it returns
	// true, the callback is removed from the pending reply queue.
	// if it returns false, the reply is considered incomplete and
	// the callback will be invoked again for the next matching reply.
	// 匹配的回复消息到达时调用回调
	// true-从挂起的回复队列中删除回调
	// false-回复不完整，并且将再次调用回调以进行下一次匹配
	callback func(resp interface{}) (done bool)

	// errc receives nil when the callback indicates completion or an
	// error if no further reply is received within the timeout.
	errc chan<- error
}

type reply struct {
	from  NodeID
	ptype byte
	data  interface{}
	// loop indicates whether there was
	// a matching request by sending on this channel.
	matched chan<- bool
}
```

接着，我们看看在table里调用的ping()是怎么实现的。

```
// ping sends a ping message to the given node and waits for a reply.
// 发送ping消息并等待回复
func (t *udp) ping(toid NodeID, toaddr *net.UDPAddr) error {
	return <-t.sendPing(toid, toaddr, nil)
}

// sendPing sends a ping message to the given node and invokes the callback
// when the reply arrives.
// 向给定节点发送ping，并在收到pang回应时调用回调
func (t *udp) sendPing(toid NodeID, toaddr *net.UDPAddr, callback func()) <-chan error {
	req := &ping{
		Version:    4,
		From:       t.ourEndpoint,
		To:         makeEndpoint(toaddr, 0), // TODO: maybe use known TCP port from DB
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	}
	packet, hash, err := encodePacket(t.priv, pingPacket, req)
	if err != nil {
		errc := make(chan error, 1)
		errc <- err
		return errc
	}
	errc := t.pending(toid, pongPacket, func(p interface{}) bool {
		ok := bytes.Equal(p.(*pong).ReplyTok, hash)
		if ok && callback != nil {
			callback()
		}
		return ok
	})
	t.write(toaddr, req.name(), packet)
	return errc
}
```

然后，顺便看下查找节点的消息发送机制。

```
// findnode sends a findnode request to the given node and waits until
// the node has sent up to k neighbors.
// 发起一个findnode请求并等待邻居节点的回应
func (t *udp) findnode(toid NodeID, toaddr *net.UDPAddr, target NodeID) ([]*Node, error) {
	// If we haven't seen a ping from the destination node for a while, it won't remember
	// our endpoint proof and reject findnode. Solicit a ping first.
	// 如果没有收到目标节点的ping，先发起ping请求
	if time.Since(t.db.lastPingReceived(toid)) > nodeDBNodeExpiration {
		t.ping(toid, toaddr)
		t.waitping(toid)
	}

	nodes := make([]*Node, 0, bucketSize)
	nreceived := 0
	errc := t.pending(toid, neighborsPacket, func(r interface{}) bool {
		reply := r.(*neighbors)
		for _, rn := range reply.Nodes {
			nreceived++
			n, err := t.nodeFromRPC(toaddr, rn)
			if err != nil {
				log.Trace("Invalid neighbor node received", "ip", rn.IP, "addr", toaddr, "err", err)
				continue
			}
			nodes = append(nodes, n)
		}
		return nreceived >= bucketSize
	})
	// 发送findnodePacket消息
	t.send(toaddr, findnodePacket, &findnode{
		Target:     target,
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	})
	return nodes, <-errc
}
...
// pending adds a reply callback to the pending reply queue.
// see the documentation of type pending for a detailed explanation.
// 将回复回调添加到挂起的回复队列
func (t *udp) pending(id NodeID, ptype byte, callback func(interface{}) bool) <-chan error {
	ch := make(chan error, 1)
	p := &pending{from: id, ptype: ptype, callback: callback, errc: ch}
	select {
	case t.addpending <- p:
		// loop will handle it
	case <-t.closing:
		ch <- errClosed
	}
	return ch
}
...
func (t *udp) send(toaddr *net.UDPAddr, ptype byte, req packet) ([]byte, error) {
	packet, hash, err := encodePacket(t.priv, ptype, req)
	if err != nil {
		return hash, err
	}
	return hash, t.write(toaddr, req.name(), packet)
}
```

那么当一个节点收到这些消息时，又是怎么处理的呢？根据代码发现四种消息包对应了四种处理方式，可见这里一定定义了一个接口来实现对收到的消息包的处理。

```
type packet interface {
	handle(t *udp, from *net.UDPAddr, fromID NodeID, mac []byte) error
	name() string
}
...
// handle ping
func (req *ping) handle(t *udp, from *net.UDPAddr, fromID NodeID, mac []byte) error {
	if expired(req.Expiration) {
		return errExpired
	}
	t.send(from, pongPacket, &pong{
		To:         makeEndpoint(from, req.From.TCP),
		ReplyTok:   mac,
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	})
	t.handleReply(fromID, pingPacket, req)

	// Add the node to the table. Before doing so, ensure that we have a recent enough pong
	// recorded in the database so their findnode requests will be accepted later.
	// 组装节点信息
	n := NewNode(fromID, from.IP, uint16(from.Port), req.From.TCP)
	// 判断上次收到pang消息的时间间隔是否超过限制
	if time.Since(t.db.lastPongReceived(fromID)) > nodeDBNodeExpiration {
		t.sendPing(fromID, from, func() { t.addThroughPing(n) })
	} else {
		t.addThroughPing(n)
	}
	t.db.updateLastPingReceived(fromID, time.Now())
	return nil
}

func (req *ping) name() string { return "PING/v4" }

// handle pong
func (req *pong) handle(t *udp, from *net.UDPAddr, fromID NodeID, mac []byte) error {
	if expired(req.Expiration) {
		return errExpired
	}
	// 没有找到匹配的ping请求
	if !t.handleReply(fromID, pongPacket, req) {
		return errUnsolicitedReply
	}
	t.db.updateLastPongReceived(fromID, time.Now())
	return nil
}

func (req *pong) name() string { return "PONG/v4" }

// handle findnode
func (req *findnode) handle(t *udp, from *net.UDPAddr, fromID NodeID, mac []byte) error {
	if expired(req.Expiration) {
		return errExpired
	}
	// 请求节点是否绑定
	if !t.db.hasBond(fromID) {
		// No endpoint proof pong exists, we don't process the packet. This prevents an
		// attack vector where the discovery protocol could be used to amplify traffic in a
		// DDOS attack. A malicious actor would send a findnode request with the IP address
		// and UDP port of the target as the source address. The recipient of the findnode
		// packet would then send a neighbors packet (which is a much bigger packet than
		// findnode) to the victim.
		return errUnknownNode
	}
	target := crypto.Keccak256Hash(req.Target[:])
	t.mutex.Lock()
	closest := t.closest(target, bucketSize).entries
	t.mutex.Unlock()

	p := neighbors{Expiration: uint64(time.Now().Add(expiration).Unix())}
	var sent bool
	// Send neighbors in chunks with at most maxNeighbors per packet
	// to stay below the 1280 byte limit.
	// 以块为单位发送最多maxNeighbors个块，以保持1280bytes的传输限制
	for _, n := range closest {
		if netutil.CheckRelayIP(from.IP, n.IP) == nil {
			p.Nodes = append(p.Nodes, nodeToRPC(n))
		}
		if len(p.Nodes) == maxNeighbors {
			t.send(from, neighborsPacket, &p)
			p.Nodes = p.Nodes[:0]
			sent = true
		}
	}
	if len(p.Nodes) > 0 || !sent {
		t.send(from, neighborsPacket, &p)
	}
	return nil
}

func (req *findnode) name() string { return "FINDNODE/v4" }

// handle neighbors
func (req *neighbors) handle(t *udp, from *net.UDPAddr, fromID NodeID, mac []byte) error {
	if expired(req.Expiration) {
		return errExpired
	}
	if !t.handleReply(fromID, neighborsPacket, req) {
		return errUnsolicitedReply
	}
	return nil
}
```

还有就是网络通信中发送的消息包都是经过RLPx编码后发送的。

```
func encodePacket(priv *ecdsa.PrivateKey, ptype byte, req interface{}) (packet, hash []byte, err error) {
	b := new(bytes.Buffer)
	b.Write(headSpace)
	b.WriteByte(ptype)
	if err := rlp.Encode(b, req); err != nil {
		log.Error("Can't encode discv4 packet", "err", err)
		return nil, nil, err
	}
	packet = b.Bytes()
	sig, err := crypto.Sign(crypto.Keccak256(packet[headSize:]), priv)
	if err != nil {
		log.Error("Can't sign discv4 packet", "err", err)
		return nil, nil, err
	}
	copy(packet[macSize:], sig)
	// add the hash to the front. Note: this doesn't protect the
	// packet in any way. Our public key will be part of this hash in
	// The future.
	hash = crypto.Keccak256(packet[macSize:])
	copy(packet, hash)
	return packet, hash, nil
}
...
func decodePacket(buf []byte) (packet, NodeID, []byte, error) {
	if len(buf) < headSize+1 {
		return nil, NodeID{}, nil, errPacketTooSmall
	}
	hash, sig, sigdata := buf[:macSize], buf[macSize:headSize], buf[headSize:]
	shouldhash := crypto.Keccak256(buf[macSize:])
	if !bytes.Equal(hash, shouldhash) {
		return nil, NodeID{}, nil, errBadHash
	}
	fromID, err := recoverNodeID(crypto.Keccak256(buf[headSize:]), sig)
	if err != nil {
		return nil, NodeID{}, hash, err
	}
	var req packet
	switch ptype := sigdata[0]; ptype {
	case pingPacket:
		req = new(ping)
	case pongPacket:
		req = new(pong)
	case findnodePacket:
		req = new(findnode)
	case neighborsPacket:
		req = new(neighbors)
	default:
		return nil, fromID, hash, fmt.Errorf("unknown type: %d", ptype)
	}
	s := rlp.NewStream(bytes.NewReader(sigdata[1:]), 0)
	err = s.Decode(req)
	return req, fromID, hash, err
}
```

### dial

上面的table类实现了Kademlia算法，udp实现了发现节点时节点间的网络通讯。发现节点后，就可以对一个节点发起连接了。devp2p中负责在两个节点建立连接的的便是dial类。

```
// NodeDialer is used to connect to nodes in the network, typically by using
// an underlying net.Dialer but also using net.Pipe in tests
// 连接网络中的节点，通常使用底层的net.Dialer，但也在测试中使用net.Pipe
type NodeDialer interface {
	Dial(*discover.Node) (net.Conn, error)
}

// TCPDialer implements the NodeDialer interface by using a net.Dialer to
// create TCP connections to nodes in the network
// 通过使用net.Dialer创建与网络中节点的TCP连接来实现NodeDialer接口
type TCPDialer struct {
	*net.Dialer
}

// Dial creates a TCP connection to the node
// 与节点创建一个tcp连接
func (t TCPDialer) Dial(dest *discover.Node) (net.Conn, error) {
	addr := &net.TCPAddr{IP: dest.IP, Port: int(dest.TCP)}
	return t.Dialer.Dial("tcp", addr.String())
}

// dialstate schedules dials and discovery lookups.
// it get's a chance to compute new tasks on every iteration
// of the main loop in Server.run.
type dialstate struct {

	// 最大的动态节点连接数
	maxDynDials int
	// discoverTable接口实现节点查询
	ntab        discoverTable
	netrestrict *netutil.Netlist

	lookupRunning bool
	// 正在连接的节点
	dialing       map[discover.NodeID]connFlag
	// 当前查询的节点结果
	lookupBuf     []*discover.Node // current discovery lookup results
	// 从k桶表随机查询的节点
	randomNodes   []*discover.Node // filled from Table
	// 静态节点
	static        map[discover.NodeID]*dialTask
	// 连接历史
	hist          *dialHistory

	// dialer首次使用的时间
	start     time.Time        // time when the dialer was first used
	// 内置节点，没有找到其他节点  连接这些节点
	bootnodes []*discover.Node // default dials when there are no peers
}

type discoverTable interface {
	Self() *discover.Node
	Close()
	Resolve(target discover.NodeID) *discover.Node
	Lookup(target discover.NodeID) []*discover.Node
	ReadRandomNodes([]*discover.Node) int
}

// the dial history remembers recent dials.
type dialHistory []pastDial

// pastDial is an entry in the dial history.
type pastDial struct {
	id  discover.NodeID
	exp time.Time
}

type task interface {
	Do(*Server)
}
```

这里有个接口定义了Do方法，同时可以看到下面有三种不同的task,可见每种task都会有对应的Do方法来处理task。

```
type task interface {
	Do(*Server)
}

// A dialTask is generated for each node that is dialed. Its
// fields cannot be accessed while the task is running.
// 每个连接的节点会生成一个dialTask
type dialTask struct {
	flags        connFlag
	dest         *discover.Node
	lastResolved time.Time
	resolveDelay time.Duration
}

// discoverTask runs discovery table operations.
// Only one discoverTask is active at any time.
// discoverTask.Do performs a random lookup.
// 发现节点任务
type discoverTask struct {
	results []*discover.Node
}

// A waitExpireTask is generated if there are no other tasks
// to keep the loop in Server.run ticking.
// 如果没有任务在server.run中循环就会生成waitExpireTask任务
type waitExpireTask struct {
	time.Duration
}
```

有三种类型的task，那么怎么来生成一个task呢？

```
// 新建一个任务
func (s *dialstate) newTasks(nRunning int, peers map[discover.NodeID]*Peer, now time.Time) []task {
	if s.start.IsZero() {
		s.start = now
	}

	var newtasks []task
	// 检查节点，然后设置状态，最后把节点加入newtasks队列
	addDial := func(flag connFlag, n *discover.Node) bool {
		if err := s.checkDial(n, peers); err != nil {
			log.Trace("Skipping dial candidate", "id", n.ID, "addr", &net.TCPAddr{IP: n.IP, Port: int(n.TCP)}, "err", err)
			return false
		}
		s.dialing[n.ID] = flag
		newtasks = append(newtasks, &dialTask{flags: flag, dest: n})
		return true
	}

	// Compute number of dynamic dials necessary at this point.
	// 计算所需的动态连接数
	needDynDials := s.maxDynDials
	// 首先统计已经建立连接的节点中动态连接数
	for _, p := range peers {
		// 动态类型
		if p.rw.is(dynDialedConn) {
			needDynDials--
		}
	}
	// 其次统计正在建立的连接的动态连接数
	for _, flag := range s.dialing {
		if flag&dynDialedConn != 0 {
			needDynDials--
		}
	}

	// Expire the dial history on every invocation.
	// 每次调用使连接记录到期
	s.hist.expire(now)

	// Create dials for static nodes if they are not connected.
	// 为所有静态节点建立连接
	for id, t := range s.static {
		err := s.checkDial(t.dest, peers)
		switch err {
		case errNotWhitelisted, errSelf:
			log.Warn("Removing static dial candidate", "id", t.dest.ID, "addr", &net.TCPAddr{IP: t.dest.IP, Port: int(t.dest.TCP)}, "err", err)
			delete(s.static, t.dest.ID)
		case nil:
			s.dialing[id] = t.flags
			newtasks = append(newtasks, t)
		}
	}
	// If we don't have any peers whatsoever, try to dial a random bootnode. This
	// scenario is useful for the testnet (and private networks) where the discovery
	// table might be full of mostly bad peers, making it hard to find good ones.
	// 当前还没有任何连接，并且fallbackInterval时间内仍未创建连接 使用内置节点
	if len(peers) == 0 && len(s.bootnodes) > 0 && needDynDials > 0 && now.Sub(s.start) > fallbackInterval {
		bootnode := s.bootnodes[0]
		s.bootnodes = append(s.bootnodes[:0], s.bootnodes[1:]...)
		s.bootnodes = append(s.bootnodes, bootnode)

		if addDial(dynDialedConn, bootnode) {
			needDynDials--
		}
	}
	// Use random nodes from the table for half of the necessary
	// dynamic dials.
	// 使用1/2的随机节点创建连接
	randomCandidates := needDynDials / 2
	if randomCandidates > 0 {
		n := s.ntab.ReadRandomNodes(s.randomNodes)
		for i := 0; i < randomCandidates && i < n; i++ {
			if addDial(dynDialedConn, s.randomNodes[i]) {
				needDynDials--
			}
		}
	}
	// Create dynamic dials from random lookup results, removing tried
	// items from the result buffer.
	// 为随机查找的节点创建动态连接，并从结果缓冲区中删除尝试的节点
	i := 0
	for ; i < len(s.lookupBuf) && needDynDials > 0; i++ {
		if addDial(dynDialedConn, s.lookupBuf[i]) {
			needDynDials--
		}
	}
	s.lookupBuf = s.lookupBuf[:copy(s.lookupBuf, s.lookupBuf[i:])]
	// Launch a discovery lookup if more candidates are needed.
	// 如果还需要更多的连接，则启动发现节点
	if len(s.lookupBuf) < needDynDials && !s.lookupRunning {
		s.lookupRunning = true
		newtasks = append(newtasks, &discoverTask{})
	}

	// Launch a timer to wait for the next node to expire if all
	// candidates have been tried and no task is currently active.
	// This should prevent cases where the dialer logic is not ticked
	// because there are no pending events.
	// 如果当前没有任何任务，创建一个waitExpireTask
	if nRunning == 0 && len(newtasks) == 0 && s.hist.Len() > 0 {
		t := &waitExpireTask{s.hist.min().exp.Sub(now)}
		newtasks = append(newtasks, t)
	}
	return newtasks
}
```

其中的checkDial方法用来检查是否需要建立连接。

```
// 检查dial状态(是否需要创建连接)
func (s *dialstate) checkDial(n *discover.Node, peers map[discover.NodeID]*Peer) error {
	_, dialing := s.dialing[n.ID]
	switch {
	case dialing:
		// 正在创建
		return errAlreadyDialing
	case peers[n.ID] != nil
		// 已经创建过连接
		return errAlreadyConnected
	case s.ntab != nil && n.ID == s.ntab.Self().ID:
		// 创建的对象不是自己
		return errSelf
	case s.netrestrict != nil && !s.netrestrict.Contains(n.IP):
		// 网络限制。对方IP不在白名单
		return errNotWhitelisted
	case s.hist.contains(n.ID):
		return errRecentlyDialed
	}
	return nil
}
```

创建任务之后，针对不同的task会有不同的Do实现。我们一个一个来看这几个task的Do处理。首先来看看dialTask的Do处理，这里的dialTask主要在两个节点之间建立连接。

```
func (t *dialTask) Do(srv *Server) {
	// 目标节点dest ip地址为空 使用resolve方法去查找目标节点并解析出ip地址
	if t.dest.Incomplete() {
		if !t.resolve(srv) {
			return
		}
	}
	// 建立连接
	err := t.dial(srv, t.dest)
	if err != nil {
		log.Trace("Dial error", "task", t, "err", err)
		// Try resolving the ID of static nodes if dialing failed.
		// 如果是静态节点连接失败，尝试重新解析其节点ip地址  因为静态节点的ip是配置的，可能发生变动
		if _, ok := err.(*dialError); ok && t.flags&staticDialedConn != 0 {
			if t.resolve(srv) {
				t.dial(srv, t.dest)
			}
		}
	}
}

// resolve attempts to find the current endpoint for the destination
// using discovery.
//
// Resolve operations are throttled with backoff to avoid flooding the
// discovery network with useless queries for nodes that don't exist.
// The backoff delay resets when the node is found.
// 当目标节点ip地址为空时使用该方法发现节点并解析ip地址
func (t *dialTask) resolve(srv *Server) bool {
	if srv.ntab == nil {
		log.Debug("Can't resolve node", "id", t.dest.ID, "err", "discovery is disabled")
		return false
	}
	if t.resolveDelay == 0 {
		t.resolveDelay = initialResolveDelay
	}
	if time.Since(t.lastResolved) < t.resolveDelay {
		return false
	}
	// 查找到节点
	resolved := srv.ntab.Resolve(t.dest.ID)
	t.lastResolved = time.Now()
	if resolved == nil {
		t.resolveDelay *= 2
		if t.resolveDelay > maxResolveDelay {
			t.resolveDelay = maxResolveDelay
		}
		log.Debug("Resolving node failed", "id", t.dest.ID, "newdelay", t.resolveDelay)
		return false
	}
	// The node was found.
	t.resolveDelay = initialResolveDelay
	t.dest = resolved
	log.Debug("Resolved node", "id", t.dest.ID, "addr", &net.TCPAddr{IP: t.dest.IP, Port: int(t.dest.TCP)})
	return true
}

type dialError struct {
	error
}

// dial performs the actual connection attempt.
// 节点连接的实现
func (t *dialTask) dial(srv *Server, dest *discover.Node) error {
	fd, err := srv.Dialer.Dial(dest)
	if err != nil {
		return &dialError{err}
	}
	// 新建一个计量连接
	mfd := newMeteredConn(fd, false)
	// 执行握手并尝试将连接方作为一个peer
	return srv.SetupConn(mfd, t.flags, dest)
}
```

这里的SetupConn方法便是上面Server对象的方法了。里面经过握手协议后将节点添加到peers队列。

看完了dialTask，接着来看看另外两个task: discoverTask和waitExpireTask的Do处理。

```
// discoverTask的Do处理
func (t *discoverTask) Do(srv *Server) {
	// newTasks generates a lookup task whenever dynamic dials are
	// necessary. Lookups need to take some time, otherwise the
	// event loop spins too fast.
	// 查找任务
	next := srv.lastLookup.Add(lookupInterval)
	if now := time.Now(); now.Before(next) {
		time.Sleep(next.Sub(now))
	}
	srv.lastLookup = time.Now()
	var target discover.NodeID
	rand.Read(target[:])
	// 查找发现节点的函数
	t.results = srv.ntab.Lookup(target)
}
...
func (t waitExpireTask) Do(*Server) {
	time.Sleep(t.Duration)
}
```

这样建立连接的代码就看完了。综上，dial通过task任务来在两个节点之间建立连接。当新建一个dialTask时会检查所有节点并设置状态，然后发起连接。如果连接的节点没有达到动态连接数时，新建discoverTask来发现更多节点。

### peer

dial对象在两个节点node之间建立连接，当节点建立连接之后便是peer。peer主要处理两个节点建立连接之后的协议处理。

```
const (
	// PeerEventTypeAdd is the type of event emitted when a peer is added
	// to a p2p.Server
	// 一个远程节点被添加到服务器
	PeerEventTypeAdd PeerEventType = "add"

	// PeerEventTypeDrop is the type of event emitted when a peer is
	// dropped from a p2p.Server
	PeerEventTypeDrop PeerEventType = "drop"

	// PeerEventTypeMsgSend is the type of event emitted when a
	// message is successfully sent to a peer
	PeerEventTypeMsgSend PeerEventType = "msgsend"

	// PeerEventTypeMsgRecv is the type of event emitted when a
	// message is received from a peer
	PeerEventTypeMsgRecv PeerEventType = "msgrecv"
)

// PeerEvent is an event emitted when peers are either added or dropped from
// a p2p.Server or when a message is sent or received on a peer connection
// Server添加或删除peer时或在peer连接上发送或接收消息时发出的事件
type PeerEvent struct {
	Type     PeerEventType   `json:"type"`
	Peer     discover.NodeID `json:"peer"`
	Error    string          `json:"error,omitempty"`
	Protocol string          `json:"protocol,omitempty"`
	MsgCode  *uint64         `json:"msg_code,omitempty"`
	MsgSize  *uint32         `json:"msg_size,omitempty"`
}

// Peer represents a connected remote node.
// 连接的远程节点
type Peer struct {
	// 节点间连接的底层信息，比如使用的socket以及对端节点支持的协议
	rw      *conn
	// 节点间生效运行的协议簇
	running map[string]*protoRW
	log     log.Logger
	created mclock.AbsTime

	wg       sync.WaitGroup
	protoErr chan error
	closed   chan struct{}
	disc     chan DiscReason

	// events receives message send / receive events if set
	// 事件接收消息发送/接收事件（如果已设置）
	events *event.Feed
}
```

这里定义了devp2p的四种消息类型，具体的协议类型参见[wiki](https://github.com/ethereum/devp2p/blob/master/devp2p.md#p2p-sub-protocol-messages)。

```
const (
	// devp2p message codes
	// 握手消息
	handshakeMsg = 0x00
	// 断开消息
	discMsg      = 0x01
	// ping
	pingMsg      = 0x02
	// ping消息的回复
	pongMsg      = 0x03
)
```

peer里至关重要的一个作用就是启动支持的协议族。

```
// 运行上层协议
func (p *Peer) run() (remoteRequested bool, err error) {
	var (
		// 写入开始的通道
		writeStart = make(chan struct{}, 1)
		writeErr   = make(chan error, 1)
		readErr    = make(chan error, 1)
		reason     DiscReason // sent to the peer
	)

	// 开启两个协程，一个用于读取，一个用于ping操作
	p.wg.Add(2)
	// readLoop协程用于接收协议数据
	go p.readLoop(readErr)
	// pingLoop协程用于保持节点在线
	go p.pingLoop()

	// Start all protocol handlers.
	// 启动协议
	writeStart <- struct{}{}
	p.startProtocols(writeStart, writeErr)

	// Wait for an error or disconnect.
	// 循环执行直到发生错误或断开
loop:
	for {
		select {
		case err = <-writeErr:
			// A write finished. Allow the next write to start if
			// there was no error.
			if err != nil {
				reason = DiscNetworkError
				break loop
			}
			writeStart <- struct{}{}
		case err = <-readErr:
			if r, ok := err.(DiscReason); ok {
				remoteRequested = true
				reason = r
			} else {
				reason = DiscNetworkError
			}
			break loop
		case err = <-p.protoErr:
			reason = discReasonForError(err)
			break loop
		case err = <-p.disc:
			reason = discReasonForError(err)
			break loop
		}
	}

	close(p.closed)
	p.rw.close(reason)
	p.wg.Wait()
	return remoteRequested, err
}
...
// 开启遍历协议
func (p *Peer) startProtocols(writeStart <-chan struct{}, writeErr chan<- error) {
	p.wg.Add(len(p.running))

	// 遍历目前运行的协议族
	for _, proto := range p.running {
		proto := proto
		proto.closed = p.closed
		proto.wstart = writeStart
		proto.werr = writeErr
		var rw MsgReadWriter = proto
		if p.events != nil {
			rw = newMsgEventer(rw, p.events, p.ID(), proto.Name)
		}
		p.log.Trace(fmt.Sprintf("Starting protocol %s/%d", proto.Name, proto.Version))
		go func() {
			// 每一个协议开启一个协程调用其Run方法
			err := proto.Run(p, rw)
			if err == nil {
				p.log.Trace(fmt.Sprintf("Protocol %s/%d returned", proto.Name, proto.Version))
				err = errProtocolReturned
			} else if err != io.EOF {
				p.log.Trace(fmt.Sprintf("Protocol %s/%d failed", proto.Name, proto.Version), "err", err)
			}
			p.protoErr <- err
			p.wg.Done()
		}()
	}
}
```

ping方法来保持节点在线很简单，这里看下另一个读取消息的协程readLoop。

```
func (p *Peer) readLoop(errc chan<- error) {
	defer p.wg.Done()
	for {

		// 接收消息
		msg, err := p.rw.ReadMsg()
		if err != nil {
			errc <- err
			return
		}

		// 消息接收时间
		msg.ReceivedAt = time.Now()
		// 处理消息
		if err = p.handle(msg); err != nil {
			errc <- err
			return
		}
	}
}
...
// 处理消息
func (p *Peer) handle(msg Msg) error {
	
	// 判断消息类型分别处理
	switch {
	// 收到ping消息，回复pong消息
	case msg.Code == pingMsg:
		msg.Discard()
		go SendItems(p.rw, pongMsg)
	case msg.Code == discMsg:
		var reason [1]DiscReason
		// This is the last message. We don't need to discard or
		// check errors because, the connection will be closed after it.
		rlp.Decode(msg.Payload, &reason)
		return reason[0]
	case msg.Code < baseProtocolLength:
		// ignore other base protocol messages
		return msg.Discard()
	default:
		// it's a subprotocol message
		// 获取子协议消息
		proto, err := p.getProto(msg.Code)
		if err != nil {
			return fmt.Errorf("msg code out of range: %v", msg.Code)
		}
		select {
		case proto.in <- msg:
			return nil
		case <-p.closed:
			return io.EOF
		}
	}
	return nil
}
...
// getProto finds the protocol responsible for handling
// the given message code.
func (p *Peer) getProto(code uint64) (*protoRW, error) {
	for _, proto := range p.running {
		if code >= proto.offset && code < proto.offset+proto.Length {
			return proto, nil
		}
	}
	return nil, newPeerError(errInvalidMsgCode, "%d", code)
}
```

peer里还有个消息读写类protoRW来实现消息的读写。

```
// 协议读写类
type protoRW struct {
	// 匿名对象
	Protocol
	// 收到消息的通道
	in     chan Msg        // receices read messages
	closed <-chan struct{} // receives when peer is shutting down
	wstart <-chan struct{} // receives when write may start
	werr   chan<- error    // for write results
	offset uint64
	w      MsgWriter
}

func (rw *protoRW) WriteMsg(msg Msg) (err error) {
	if msg.Code >= rw.Length {
		return newPeerError(errInvalidMsgCode, "not handled")
	}
	msg.Code += rw.offset
	select {
	case <-rw.wstart:
		err = rw.w.WriteMsg(msg)
		// Report write status back to Peer.run. It will initiate
		// shutdown if the error is non-nil and unblock the next write
		// otherwise. The calling protocol code should exit for errors
		// as well but we don't want to rely on that.
		rw.werr <- err
	case <-rw.closed:
		err = ErrShuttingDown
	}
	return err
}

func (rw *protoRW) ReadMsg() (Msg, error) {
	select {
	case msg := <-rw.in:
		msg.Code -= rw.offset
		return msg, nil
	case <-rw.closed:
		return Msg{}, io.EOF
	}
}
```

然后Protocol类定义了P2P协议；Rlpx定义了P2P网络通信底层的消息加密方式，peer中建立连接的两次握手就都是在这实现的。


至此p2p的核心源码就大致看完了。首先通过discover目录下的各个类去发现周围节点并将它们存储到数据库，这里主要涉及Kademlia算法的理解和实现。接着，通过dial在两个节点之间建立连接。随后建立连接的网络链路两端的节点就是peer，peer之间通过支持的协议进行通讯，建立tcp连接进行消息的传递。其中，底层的消息传递通过RLPx Encayption进行加密传输。







