// Copyright 2017 The go-ethereum Authors
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

package ethash

import (
	crand "crypto/rand"
	"math"
	"math/big"
	"math/rand"
	"runtime"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

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
		// 难度目标值
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
