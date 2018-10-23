# 0x04 RLP源码解析

RLP(Recursive Length Prefix)，递归长度前缀编码，它是以太坊序 化所采取的编码方式。RLP主要用于以太坊中数据的网络传输和持久化存储。

# RLP理解

## RLP编码

RLP编码针对的数据类型主要有两种：

- byte数组
- byte数组的数组，即列表

设定Rc(x)为RLP编码函数。
首先来看针对byte数组的几个编码规则：

> **1.针对单字节b，b ∈ [0,127], Rc(b) = b**

**eg:**Rc(a) = 97, Rc(w) = 119 

> **2.针对字节数组bytes，length(bytes) <= 55, Rc(bytes) = 128+length(bytes) ++ bytes(++符号表示拼接)**

**eg:**Rc(abc) = [131 97 98 99], 其中131 = 128+length(abc), [97 98 99]为[abc]本身编码

>**3.针对字节数组bytes，length(bytes) > 55,
Rc(bytes) = 183+sizeof(sizeof(bytes)) ++ Rc(sizeof(bytes)) + bytes**

__eg:__ str = "The length of this sentence is more than 55 bytes, I know it because I pre-designed it" 
Rc(str) = [**184** **86** 84 104 101 32 108 101 110 103 116 104 32 111 102 32 116 104 105 115 32 115 101 110 116 101 110 99 101 32 105 115 32 109 111 114 101 32 116 104 97 110 32 53 53 32 98 121 116 101 115 44 32 73 32 107 110 111 119 32 105 116 32 98 101 99 97 117 115 101 32 73 32 112 114 101 45 100 101 115 105 103 110 101 100 32 105 116]
经计算该字符串占用字节数为86，显然184 = 183 + sizeof(86)，86 = sizeof(str), 84为“T”的编码，从84开始后面便是str本身的编码。

###### 以上是针对字节数组，接下来看以字节数组为元素的数组，这里称之为列表的编码方式：

首先要明确几个概念，针对一个列表list，lenL是指list内每个bytes字节数的总和，lenDl是指list每个bytes经过Rc(x)编码后的总长度。

> **lenL(list) = $\sum_{i=0}^Nsizeof(bytes_i)$
lenDl(list) = $\sum_{i=0}^Nsizeof(Rc(bytes_i))$**

>**4.针对list[bytes0, bytes1...bytesN]，lenL(list) <= 55, 
Rc(list) = 192+lenDl(list) ++ Rc($bytes_i$)**

__eg:__ list = ["abc", "def"],
Rc(list) = [**200** 131 97 98 99 131 100 101 102]
首先，lenL(list) = 3+3 <= 55,
然后，根据**规则3**可以得出：
Rc[abc] = [**131** 97 98 99]， 
Rc[def] = [**131** 100 101 102](128+3 100 101 102) 
现在就知道，lenDl(list) = 4 + 4 = 8，所以开始的200就是192+8的结构，后面跟的是list里每个bytes的RLP编码。

> **5.针对list[bytes0, bytes1...bytesN]，lenL(list) > 55,
Rc(list) = 247+sizeof(lenDl(list)) ++ lenDl(list) ++ Rc($bytes_i$)**

__eg:__ list = ["The length of this sentence is more than 55 bytes,", 
" I know it because I pre-designed it"]
Rc(list) = [**248 88 179** 84 104 101 32 108 101 110 103 116 104 32 111 102 32 116 104 105 115 32 115 101 110 116 101 110 99 101 32 105 115 32 109 111 114 101 32 116 104 97 110 32 53 53 32 98 121 116 101 115 44 32 **163** 73 32 107 110 111 119 32 105 116 32 98 101 99 97 117 115 101 32 73 32 112 114 101 45 100 101 115 105 103 110 101 100 32 105 116]
首先，lenL(list) = 51+35 = 86 > 55,
然后，根据**规则2**可以得出：
Rc[The length of this sentence is more than 55 bytes,] = [**179** 84 104 101 32 108 101 110 103 116 104 32 111 102 32 116 104 105 115 32 115 101 110 116 101 110 99 101 32 105 115 32 109 111 114 101 32 116 104 97 110 32 53 53 32 98 121 116 101 115 44 32](179 = 128 + 51)， 
Rc[ I know it because I pre-designed it] 
= [**163** 73 32 107 110 111 119 32 105 116 32 98 101 99 97 117 115 101 32 73 32 112 114 101 45 100 101 115 105 103 110 101 100 32 105 116](163 = 128 + 35) 
现在就知道，lenDl(list) = 52 + 36 = 88，88只需占1个字节即可，所以开始的248就是247+1的结构，后面的88是lenDl(list)本身的编码，再后面跟的是list里每个bytes的RLP编码。

## RLP解码

我们知道，有编码就有解码。编码是解码的一个逆过程，我们发现在编码的时候不同的情况都有一个不同的字节前缀，所以解码的时候也是从这个字节前缀入手。

设定Dr(x)为解码函数，s为x的第一个字节。
subX[m,n]表示x的子串，从x的第m个开始取n个字节。
bin2Int(bytes)表示将一个字节数组按BigEndian编码转换为整数，目的在于求出被编码字符串长度。

> **1. s ∈ [0, 127], 对应编码规则1单字节解码，
Dr(x) = x**

> **2. s ∈ [128, 184), 对应编码规则2，数组长度不超过55    
Dr(x) = subX[2, s-128]**

> **3. s ∈ [184, 192), 对应编码规则3，数组长度超过55 
bin2Int(subX[1, s-183])为被编码数组的长度 
Dr(x) = subX[bin2Int(subX[1, s-183]+1, bin2Int(subX[1, s-183])]**

> **4. s ∈ [192, 247), 对应编码规则4，总长不超过55的列表 列表总长lenDl = s-192，然后递归调用解码规则1-3即可
Dr(x) = $\sum_{i=0}^ NspliceOf(Dr(x_i))$**

> **5. s ∈ [247, 256], 对应编码规则5，总长超过55的列表,列表总长lenDl = bin2Int(subX[1, s-247])，然后递归调用解码规则1-3即可
Dr(x) = $\sum_{i=0}^ NspliceOf(Dr(x_i))$**

# RLP源码

上面大概了解了RLP的编解码方式，下面我们就到eth源代码中去看看有关RLP的代码是怎么写的。

上篇文章主要介绍了geth源码结构，RLP源码主要在rlp目录下。

![RLP源码结构](https://upload-images.jianshu.io/upload_images/830585-a903616762ac1d8e.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

### typeCache.go

该结构主要实现不同类型和对应编解码器的映射关系，通过它去获取对应的编解码器。

核心数据结构
```
// 核心数据结构
var (
	typeCacheMutex sync.RWMutex    // 读写锁，用于在多线程时保护typeCache
	typeCache      = make(map[typekey]*typeinfo)  // 保存类型 -> 编码器函数的数据结构
    // Map的key是类型，value是对应的编码和解码器
)

type typeinfo struct {
	decoder    // 解码器函数
	writer     // 编码器函数
}
```

如何获取编码器和解码器的函数

```
// 获取编码器和解码器的函数
func cachedTypeInfo(typ reflect.Type, tags tags) (*typeinfo, error) {
	typeCacheMutex.RLock()    // 加锁保护
	info := typeCache[typekey{typ, tags}]    // 将传入的typ和tags封装为typekey类型
	typeCacheMutex.RUnlock()  // 解锁
	if info != nil {    // 成功获取到typ对应的编解码函数
		return info, nil
	}
	// not in the cache, need to generate info for this type.
	// 编解码不在typeCache中，需要创建该typ对应的编解码函数
	typeCacheMutex.Lock()
	defer typeCacheMutex.Unlock()
	return cachedTypeInfo1(typ, tags)
}

// 新建typ对应的编解码函数
func cachedTypeInfo1(typ reflect.Type, tags tags) (*typeinfo, error) {
	key := typekey{typ, tags}
	info := typeCache[key]
	if info != nil {
		// another goroutine got the write lock first
		/// 其他线程已经成功创建
		return info, nil
	}
	// put a dummmy value into the cache before generating.
	// if the generator tries to lookup itself, it will get
	// the dummy value and won't call itself recursively.
	// 这个地方首先创建了一个值来填充这个类型的位置，避免遇到一些递归定义的数据类型形成死循环
	typeCache[key] = new(typeinfo)
	info, err := genTypeInfo(typ, tags)    // 生成对应类型的编解码器函数
	if err != nil {
		// remove the dummy value if the generator fails
		// 创建失败处理
		delete(typeCache, key)
		return nil, err
	}
	*typeCache[key] = *info
	return typeCache[key], err
}
```
生成对应编解码器的函数

```

// 生成对应编解码器的函数
func genTypeInfo(typ reflect.Type, tags tags) (info *typeinfo, err error) {
	info = new(typeinfo)
	if info.decoder, err = makeDecoder(typ, tags); err != nil {
		return nil, err
	}
	if info.writer, err = makeWriter(typ, tags); err != nil {
		return nil, err
	}
	return info, nil
}
```

### decode.go

typeCache定义了类型与对应解编码器的映射关系，接下来就看看对应的编码和解码代码。

```
// 定义一些解码错误
var (
	// EOL is returned when the end of the current list
	// has been reached during streaming.
	EOL = errors.New("rlp: end of list")

	// Actual Errors
	ErrExpectedString   = errors.New("rlp: expected String or Byte")
	ErrExpectedList     = errors.New("rlp: expected List")
	ErrCanonInt         = errors.New("rlp: non-canonical integer format")
	ErrCanonSize        = errors.New("rlp: non-canonical size information")
	ErrElemTooLarge     = errors.New("rlp: element is larger than containing list")
	ErrValueTooLarge    = errors.New("rlp: value size exceeds available input length")
	ErrMoreThanOneValue = errors.New("rlp: input contains more than one value")

	// internal errors
	errNotInList     = errors.New("rlp: call of ListEnd outside of any list")
	errNotAtEOL      = errors.New("rlp: call of ListEnd not positioned at EOL")
	errUintOverflow  = errors.New("rlp: uint overflow")
	errNoPointer     = errors.New("rlp: interface given to Decode must be a pointer")
	errDecodeIntoNil = errors.New("rlp: pointer given to Decode must not be nil")
)
```

```
// 解码器  根据不同的类型返回对应的解码器函数
func makeDecoder(typ reflect.Type, tags tags) (dec decoder, err error) {
	kind := typ.Kind()
	switch {
	case typ == rawValueType:
		return decodeRawValue, nil
	case typ.Implements(decoderInterface):
		return decodeDecoder, nil
	case kind != reflect.Ptr && reflect.PtrTo(typ).Implements(decoderInterface):
		return decodeDecoderNoPtr, nil
	case typ.AssignableTo(reflect.PtrTo(bigInt)):
		return decodeBigInt, nil
	case typ.AssignableTo(bigInt):
		return decodeBigIntNoPtr, nil
	case isUint(kind):
		return decodeUint, nil
	case kind == reflect.Bool:
		return decodeBool, nil
	case kind == reflect.String:
		return decodeString, nil
	case kind == reflect.Slice || kind == reflect.Array:
		return makeListDecoder(typ, tags)
	case kind == reflect.Struct:
		return makeStructDecoder(typ)
	case kind == reflect.Ptr:
		if tags.nilOK {
			return makeOptionalPtrDecoder(typ)
		}
		return makePtrDecoder(typ)
	case kind == reflect.Interface:
		return decodeInterface, nil
	default:
		return nil, fmt.Errorf("rlp: type %v is not RLP-serializable", typ)
	}
}
```

根据以上switch方法可以调到各种解码函数。

### encode.go

接下来来看看有关编码的代码解读。首先定义了两种特殊情况下的编码方式。

```
var (
	// Common encoded values.
	// These are useful when implementing EncodeRLP.
	EmptyString = []byte{0x80}		// 针对空字符串的编码
	EmptyList   = []byte{0xC0}		// 针对空列表的编码
)
```

接着定义了一个接口EncodeRLP供调用，但是很多情况下EncodeRLP会调用Encode方法。

```
type Encoder interface {
	// EncodeRLP should write the RLP encoding of its receiver to w.
	// If the implementation is a pointer method, it may also be
	// called for nil pointers.
	//
	// Implementations should generate valid RLP. The data written is
	// not verified at the moment, but a future version might. It is
	// recommended to write only a single value but writing multiple
	// values or no value at all is also permitted.
	EncodeRLP(io.Writer) error
}
```

```
func Encode(w io.Writer, val interface{}) error {
	if outer, ok := w.(*encbuf); ok {
		// Encode was called by some type's EncodeRLP.
		// Avoid copying by writing to the outer encbuf directly.
		return outer.encode(val)
	}
	eb := encbufPool.Get().(*encbuf)	// 获取一个编码缓冲区encbuf
	defer encbufPool.Put(eb)
	eb.reset()
	if err := eb.encode(val); err != nil {
		return err
	}
	return eb.toWriter(w)
}
```

```
func (w *encbuf) encode(val interface{}) error {
	rval := reflect.ValueOf(val)
	ti, err := cachedTypeInfo(rval.Type(), tags{})
	if err != nil {
		return err
	}
	return ti.writer(rval, w)
}
```

makeWriter作为一个switch形式的编码函数和上面的makeDecoder类似。


.
.
.
.
>###互联网颠覆世界，区块链颠覆互联网!

>###### --------------------------------------------------20180911 00:21


