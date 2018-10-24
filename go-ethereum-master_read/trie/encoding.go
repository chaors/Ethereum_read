// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

// Trie keys are dealt with in three distinct encodings:
//
// KEYBYTES encoding contains the actual key and nothing else. This encoding is the
// input to most API functions.
//
// HEX encoding contains one byte for each nibble of the key and an optional trailing
// 'terminator' byte of value 0x10 which indicates whether or not the node at the key
// contains a value. Hex key encoding is used for nodes loaded in memory because it's
// convenient to access.
//
// COMPACT encoding is defined by the Ethereum Yellow Paper (it's called "hex prefix
// encoding" there) and contains the bytes of the key and a flag. The high nibble of the
// first byte contains the flag; the lowest bit encoding the oddness of the length and
// the second-lowest encoding whether the node at the key is a value node. The low nibble
// of the first byte is zero in the case of an even number of nibbles and the first nibble
// in the case of an odd number. All remaining nibbles (now an even number) fit properly
// into the remaining bytes. Compact encoding is used for nodes stored on disk.
// Hex编码串转化为Compact编码
func hexToCompact(hex []byte) []byte {
	// 如果最后一位是16，terminator为1，否则为0
	terminator := byte(0)
	// 包含terminator的节点为叶子节点
	if hasTerm(hex) {
		terminator = 1
		// 1.0将Hex格式的尾部标记byte去掉
		hex = hex[:len(hex)-1]
	}
	// 定义Compat字节数组
	buf := make([]byte, len(hex)/2+1)
	// 标志位默认
	buf[0] = terminator << 5 // the flag byte
	if len(hex)&1 == 1 {
		// 如果Hex长度为奇数，修改标志位为odd flag
		buf[0] |= 1 << 4 // odd flag
		// 然后把第1个nibble放入buf[0]低四位
		buf[0] |= hex[0] // first nibble is contained in the first byte
		hex = hex[1:]
	}
	// 1.1然后将每2nibble的数据合并到1个byte
	decodeNibbles(hex, buf[1:])
	return buf
}

// Compact编码转化为Hex编码串
func compactToHex(compact []byte) []byte {
	base := keybytesToHex(compact)
	// delete terminator flag

	/*这里base[0]有4中情况
	  00000000	扩展节点偶数位
	  00000001	扩展节点奇数位
	  00000010	叶子节点偶数位
	  00000011	叶子节点偶数位
	*/

	if base[0] < 2 {
		// 如果是扩展节点，去除最后一位
		base = base[:len(base)-1]
	}
	// apply odd flag
	// 如果是偶数位chop=2，否则chop=1
	chop := 2 - base[0]&1
	//去除compact标志位。偶数位去除2个字节，奇数位去除1个字节（因为奇数位的低四位放的是nibble数据）
	return base[chop:]
}

// 将key字符串进行Hex编码
func keybytesToHex(str []byte) []byte {
	l := len(str)*2 + 1
	//将一个keybyte转化成两个字节
	var nibbles = make([]byte, l)
	for i, b := range str {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}
	//末尾加入Hex标志位16 00010000
	nibbles[l-1] = 16
	return nibbles
}

// hexToKeybytes turns hex nibbles into key bytes.
// This can only be used for keys of even length.
// 将hex编码解码转为key字符串
func hexToKeybytes(hex []byte) []byte {
	if hasTerm(hex) {
		hex = hex[:len(hex)-1]
	}
	if len(hex)&1 != 0 {
		panic("can't convert hex key of odd length")
	}
	key := make([]byte, len(hex)/2)
	decodeNibbles(hex, key)
	return key
}

func decodeNibbles(nibbles []byte, bytes []byte) {
	for bi, ni := 0, 0; ni < len(nibbles); bi, ni = bi+1, ni+2 {
		bytes[bi] = nibbles[ni]<<4 | nibbles[ni+1]
	}
}

// prefixLen returns the length of the common prefix of a and b.
func prefixLen(a, b []byte) int {
	var i, length = 0, len(a)
	if len(b) < length {
		length = len(b)
	}
	for ; i < length; i++ {
		if a[i] != b[i] {
			break
		}
	}
	return i
}

// hasTerm returns whether a hex key has the terminator flag.
// 是否包含Hex格式标识符(末尾byte为16 00010000)
func hasTerm(s []byte) bool {
	return len(s) > 0 && s[len(s)-1] == 16
}
