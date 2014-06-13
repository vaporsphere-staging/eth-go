package ethchain

import (
	"fmt"
	"github.com/ethereum/eth-go/ethutil"
	"math/big"
)

type StateTransition struct {
	coinbase []byte
	tx       *Transaction
	gas      *big.Int
	state    *State
	block    *Block

	cb, rec, sen *StateObject
}

func NewStateTransition(coinbase []byte, tx *Transaction, state *State, block *Block) *StateTransition {
	return &StateTransition{coinbase, tx, new(big.Int), state, block, nil, nil, nil}
}

func (self *StateTransition) Coinbase() *StateObject {
	if self.cb != nil {
		return self.cb
	}

	self.cb = self.state.GetAccount(self.coinbase)
	return self.cb
}
func (self *StateTransition) Sender() *StateObject {
	if self.sen != nil {
		return self.sen
	}

	self.sen = self.state.GetAccount(self.tx.Sender())
	return self.sen
}
func (self *StateTransition) Receiver() *StateObject {
	if self.tx.CreatesContract() {
		return nil
	}

	if self.rec != nil {
		return self.rec
	}

	self.rec = self.state.GetAccount(self.tx.Recipient)
	return self.rec
}

func (self *StateTransition) MakeStateObject(state *State, tx *Transaction) *StateObject {
	contract := MakeContract(tx, state)
	if contract != nil {
		state.states[string(tx.CreationAddress())] = contract.state

		return contract
	}

	return nil
}

func (self *StateTransition) UseGas(amount *big.Int) error {
	if self.gas.Cmp(amount) < 0 {
		return OutOfGasError()
	}
	self.gas.Sub(self.gas, amount)

	return nil
}

func (self *StateTransition) AddGas(amount *big.Int) {
	self.gas.Add(self.gas, amount)
}

func (self *StateTransition) BuyGas() error {
	var err error

	sender := self.Sender()
	if sender.Amount.Cmp(self.tx.GasValue()) < 0 {
		return fmt.Errorf("Insufficient funds to pre-pay gas. Req %v, has %v", self.tx.GasValue(), self.tx.Value)
	}

	coinbase := self.Coinbase()
	err = coinbase.BuyGas(self.tx.Gas, self.tx.GasPrice)
	if err != nil {
		return err
	}
	self.state.UpdateStateObject(coinbase)

	self.AddGas(self.tx.Gas)
	sender.SubAmount(self.tx.GasValue())

	return nil
}

func (self *StateTransition) TransitionState() (err error) {
	//snapshot := st.state.Snapshot()

	defer func() {
		if r := recover(); r != nil {
			ethutil.Config.Log.Infoln(r)
			err = fmt.Errorf("%v", r)
		}
	}()

	var (
		tx       = self.tx
		sender   = self.Sender()
		receiver *StateObject
	)

	if sender.Nonce != tx.Nonce {
		return NonceError(tx.Nonce, sender.Nonce)
	}

	sender.Nonce += 1
	defer func() {
		// Notify all subscribers
		//self.Ethereum.Reactor().Post("newTx:post", tx)
	}()

	if err = self.BuyGas(); err != nil {
		return err
	}

	receiver = self.Receiver()

	if err = self.UseGas(GasTx); err != nil {
		return err
	}

	dataPrice := big.NewInt(int64(len(tx.Data)))
	dataPrice.Mul(dataPrice, GasData)
	if err = self.UseGas(dataPrice); err != nil {
		return err
	}

	if receiver == nil { // Contract
		receiver = self.MakeStateObject(self.state, tx)
		if receiver == nil {
			return fmt.Errorf("ERR. Unable to create contract with transaction %v", tx)
		}
	}

	if err = self.transferValue(sender, receiver); err != nil {
		return err
	}

	if tx.CreatesContract() {
		fmt.Println(Disassemble(receiver.Init()))
		// Evaluate the initialization script
		// and use the return value as the
		// script section for the state object.
		//script, gas, err = sm.Eval(state, contract.Init(), contract, tx, block)
		code, err := self.Eval(receiver.Init(), receiver)
		if err != nil {
			return fmt.Errorf("Error during init script run %v", err)
		}

		receiver.script = code
	}

	self.state.UpdateStateObject(sender)
	self.state.UpdateStateObject(receiver)

	return nil
}

func (self *StateTransition) transferValue(sender, receiver *StateObject) error {
	if sender.Amount.Cmp(self.tx.Value) < 0 {
		return fmt.Errorf("Insufficient funds to transfer value. Req %v, has %v", self.tx.Value, sender.Amount)
	}

	// Subtract the amount from the senders account
	sender.SubAmount(self.tx.Value)
	// Add the amount to receivers account which should conclude this transaction
	receiver.AddAmount(self.tx.Value)

	ethutil.Config.Log.Debugf("%x => %x (%v) %x\n", sender.Address()[:4], receiver.Address()[:4], self.tx.Value, self.tx.Hash())

	return nil
}

func (self *StateTransition) Eval(script []byte, context *StateObject) (ret []byte, err error) {
	var (
		tx        = self.tx
		block     = self.block
		initiator = self.Sender()
		state     = self.state
	)

	closure := NewClosure(initiator, context, script, state, self.gas, tx.GasPrice)
	vm := NewVm(state, nil, RuntimeVars{
		Origin:      initiator.Address(),
		BlockNumber: block.BlockInfo().Number,
		PrevHash:    block.PrevHash,
		Coinbase:    block.Coinbase,
		Time:        block.Time,
		Diff:        block.Difficulty,
		Value:       tx.Value,
	})
	ret, _, err = closure.Call(vm, tx.Data, nil)

	return
}