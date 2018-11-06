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

package clique

import (
	"bytes"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	lru "github.com/hashicorp/golang-lru"
)

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

// newSnapshot creates a new snapshot with the specified startup parameters. This
// method does not initialize the set of recent signers, so only ever use if for
// the genesis block.
func newSnapshot(config *params.CliqueConfig, sigcache *lru.ARCCache, number uint64, hash common.Hash, signers []common.Address) *Snapshot {
	snap := &Snapshot{
		config:   config,
		sigcache: sigcache,
		Number:   number,
		Hash:     hash,
		Signers:  make(map[common.Address]struct{}),
		Recents:  make(map[uint64]common.Address),
		Tally:    make(map[common.Address]Tally),
	}
	for _, signer := range signers {
		snap.Signers[signer] = struct{}{}
	}
	return snap
}

// loadSnapshot loads an existing snapshot from the database.
func loadSnapshot(config *params.CliqueConfig, sigcache *lru.ARCCache, db ethdb.Database, hash common.Hash) (*Snapshot, error) {
	blob, err := db.Get(append([]byte("clique-"), hash[:]...))
	if err != nil {
		return nil, err
	}
	snap := new(Snapshot)
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}
	snap.config = config
	snap.sigcache = sigcache

	return snap, nil
}

// store inserts the snapshot into the database.
func (s *Snapshot) store(db ethdb.Database) error {
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Put(append([]byte("clique-"), s.Hash[:]...), blob)
}

// copy creates a deep copy of the snapshot, though not the individual votes.
func (s *Snapshot) copy() *Snapshot {
	cpy := &Snapshot{
		config:   s.config,
		sigcache: s.sigcache,
		Number:   s.Number,
		Hash:     s.Hash,
		Signers:  make(map[common.Address]struct{}),
		Recents:  make(map[uint64]common.Address),
		Votes:    make([]*Vote, len(s.Votes)),
		Tally:    make(map[common.Address]Tally),
	}
	for signer := range s.Signers {
		cpy.Signers[signer] = struct{}{}
	}
	for block, signer := range s.Recents {
		cpy.Recents[block] = signer
	}
	for address, tally := range s.Tally {
		cpy.Tally[address] = tally
	}
	copy(cpy.Votes, s.Votes)

	return cpy
}

// validVote returns whether it makes sense to cast the specified vote in the
// given snapshot context (e.g. don't try to add an already authorized signer).
func (s *Snapshot) validVote(address common.Address, authorize bool) bool {
	_, signer := s.Signers[address]
	return (signer && !authorize) || (!signer && authorize)
}

// cast adds a new vote into the tally.
func (s *Snapshot) cast(address common.Address, authorize bool) bool {
	// Ensure the vote is meaningful
	if !s.validVote(address, authorize) {
		return false
	}
	// Cast the vote into an existing or new tally
	if old, ok := s.Tally[address]; ok {
		old.Votes++
		s.Tally[address] = old
	} else {
		s.Tally[address] = Tally{Authorize: authorize, Votes: 1}
	}
	return true
}

// uncast removes a previously cast vote from the tally.
func (s *Snapshot) uncast(address common.Address, authorize bool) bool {
	// If there's no tally, it's a dangling vote, just drop
	tally, ok := s.Tally[address]
	if !ok {
		return false
	}
	// Ensure we only revert counted votes
	if tally.Authorize != authorize {
		return false
	}
	// Otherwise revert the vote
	if tally.Votes > 1 {
		tally.Votes--
		s.Tally[address] = tally
	} else {
		delete(s.Tally, address)
	}
	return true
}

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

// signers retrieves the list of authorized signers in ascending order.
func (s *Snapshot) signers() []common.Address {
	signers := make([]common.Address, 0, len(s.Signers))
	for signer := range s.Signers {
		signers = append(signers, signer)
	}
	for i := 0; i < len(signers); i++ {
		for j := i + 1; j < len(signers); j++ {
			if bytes.Compare(signers[i][:], signers[j][:]) > 0 {
				signers[i], signers[j] = signers[j], signers[i]
			}
		}
	}
	return signers
}

// inturn returns if a signer at a given block height is in-turn or not.
// 给定高度块的签名者是否轮流
func (s *Snapshot) inturn(number uint64, signer common.Address) bool {
	signers, offset := s.signers(), 0
	for offset < len(signers) && signers[offset] != signer {
		offset++
	}
	return (number % uint64(len(signers))) == uint64(offset)
}
