package new_vm

import (
	"context"

	"sync"

	"github.com/iost-official/Go-IOS-Protocol/core/block"
	"github.com/iost-official/Go-IOS-Protocol/core/contract"
	"github.com/iost-official/Go-IOS-Protocol/core/new_tx"
	"github.com/iost-official/Go-IOS-Protocol/new_vm/database"
)

const (
	defaultCacheLength = 1000
)

type Engine interface {
	//Init()
	//SetEnv(bh *block.BlockHead, cb database.IMultiValue) Engine
	Exec(tx0 tx.Tx) (tx.TxReceipt, error)
	GC()
}

var staticMonitor *Monitor

var once sync.Once

type EngineImpl struct {
	host *Host
}

//func (e *EngineImpl) Init() {
//	if staticMonitor == nil {
//		once.Do(func() {
//			staticMonitor = NewMonitor()
//		})
//	}
//}

func NewEngine(bh *block.BlockHead, cb database.IMultiValue) Engine {
	if staticMonitor == nil {
		once.Do(func() {
			staticMonitor = NewMonitor()
		})
	}

	ctx := context.Background()
	db := database.NewVisitor(defaultCacheLength, cb)
	host := NewHost(ctx, db)
	return &EngineImpl{host: host}
}
func (e *EngineImpl) Exec(tx0 tx.Tx) (tx.TxReceipt, error) {

	txr := tx.NewTxReceipt(tx0.Hash())
	totalCost := contract.Cost0()

	for _, action := range tx0.Actions {
		_, receipt, cost, err := staticMonitor.Call(e.host, action.Contract, action.ActionName, action.Data)
		if err != nil {
			return txr, err
		}
		txr.Receipts = append(txr.Receipts, *receipt)
		totalCost.AddAssign(cost)
	}

	//e.host.PayCost(totalCost, tx0.Publisher.) todo pay cost

	return txr, nil
}
func (e *EngineImpl) GC() {

}
