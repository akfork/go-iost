package chainbase

import (
	"errors"
	"time"

	"github.com/iost-official/go-iost/common"
	"github.com/iost-official/go-iost/core/block"
	"github.com/iost-official/go-iost/core/blockcache"
	"github.com/iost-official/go-iost/ilog"
)

var (
	errSingle     = errors.New("single block")
	errDuplicate  = errors.New("duplicate block")
	errOutOfLimit = errors.New("block out of limit in one slot")
)

// Block will describe the block of chainbase.
type Block struct {
	*block.Block
	*blockcache.WitnessList
	Irreversible bool
}

// HeadBlock will return the head block of chainbase.
func (c *ChainBase) HeadBlock() *Block {
	head := c.bCache.Head()
	block := &Block{
		Block:        head.Block,
		WitnessList:  &head.WitnessList,
		Irreversible: false,
	}
	return block
}

// LIBlock will return the last irreversible block of chainbase.
func (c *ChainBase) LIBlock() *Block {
	lib := c.bCache.LinkedRoot()
	block := &Block{
		Block:        lib.Block,
		WitnessList:  &lib.WitnessList,
		Irreversible: true,
	}
	return block
}

// GetBlockByHash will return the block by hash.
// If block is not exist, it will return nil and false.
func (c *ChainBase) GetBlockByHash(hash []byte) (*Block, bool) {
	block, err := c.bCache.GetBlockByHash(hash)
	if err != nil {
		block, err := c.bChain.GetBlockByHash(hash)
		if err != nil {
			ilog.Warnf("Get block by hash %v failed: %v", common.Base58Encode(hash), err)
			return nil, false
		}
		return &Block{
			Block:        block,
			Irreversible: true,
		}, true
	}
	return &Block{
		Block:        block,
		Irreversible: false,
	}, true
}

// GetBlockHashByNum will return the block hash by number.
// If block hash is not exist, it will return nil and false.
func (c *ChainBase) GetBlockHashByNum(num int64) ([]byte, bool) {
	var hash []byte
	if blk, err := c.bCache.GetBlockByNumber(num); err != nil {
		hash, err = c.bChain.GetHashByNumber(num)
		if err != nil {
			ilog.Debugf("Get hash by num %v failed: %v", num, err)
			return nil, false
		}
	} else {
		hash = blk.HeadHash()
	}
	return hash, true
}

func (c *ChainBase) printStatistics(num int64, blk *block.Block, replay bool, gen bool) {
	action := "Recover"
	if !replay {
		if gen {
			action = "Generate"
		} else {
			action = "Receive"
		}

	}
	ptx, _ := c.txPool.PendingTx()
	ilog.Infof("%v block - @%v id:%v..., t:%v, num:%v, confirmed:%v, txs:%v, pendingtxs:%v, et:%dms",
		action,
		num,
		blk.Head.Witness[:10],
		blk.Head.Time,
		blk.Head.Number,
		c.bCache.LinkedRoot().Head.Number,
		len(blk.Txs),
		ptx.Size(),
		(time.Now().UnixNano()-blk.Head.Time)/1e6,
	)
}

// Add will add a block to block cache and verify it.
func (c *ChainBase) Add(blk *block.Block, replay bool, gen bool) error {
	_, err := c.bCache.Find(blk.HeadHash())
	if err == nil {
		return errDuplicate
	}

	err = blk.VerifySelf()
	if err != nil {
		ilog.Warnf("Verify block basics failed: %v", err)
		return err
	}

	parent, err := c.bCache.Find(blk.Head.ParentHash)
	c.bCache.Add(blk)
	if err == nil && parent.Type == blockcache.Linked {
		err := c.addExistingBlock(blk, parent, replay, gen)
		if err != nil {
			ilog.Warnf("Verify block execute failed: %v", err)
		}
		return err
	}
	return errSingle
}

func (c *ChainBase) addExistingBlock(blk *block.Block, parentNode *blockcache.BlockCacheNode, replay bool, gen bool) error {
	node, _ := c.bCache.Find(blk.HeadHash())

	if parentNode.Block.Head.Witness != blk.Head.Witness ||
		common.SlotOfUnixNano(parentNode.Block.Head.Time) != common.SlotOfUnixNano(blk.Head.Time) {
		node.SerialNum = 0
	} else {
		node.SerialNum = parentNode.SerialNum + 1
	}

	if node.SerialNum >= int64(common.BlockNumPerWitness) {
		return errOutOfLimit
	}
	ok := c.stateDB.Checkout(string(blk.HeadHash()))
	if !ok {
		c.stateDB.Checkout(string(blk.Head.ParentHash))
		err := verifyBlock(blk, parentNode.Block, &node.GetParent().WitnessList, c.txPool, c.stateDB, c.bChain, replay)
		if err != nil {
			ilog.Errorf("verify block failed, blockNum:%v, blockHash:%v. err=%v", blk.Head.Number, common.Base58Encode(blk.HeadHash()), err)
			c.bCache.Del(node)
			return err
		}
		c.stateDB.Commit(string(blk.HeadHash()))
	}
	c.bCache.Link(node, replay)
	c.bCache.UpdateLib(node)
	// After UpdateLib, the block head active witness list will be right
	// So AddLinkedNode need execute after UpdateLib
	c.txPool.AddLinkedNode(node)
	if replay {
		ilog.Infof("node %d %s active list: %v", node.Head.Number, common.Base58Encode(node.HeadHash()), node.Active())
	}

	c.printStatistics(node.SerialNum, node.Block, replay, gen)

	for child := range node.Children {
		c.addExistingBlock(child.Block, node, replay, gen)
	}
	return nil
}
