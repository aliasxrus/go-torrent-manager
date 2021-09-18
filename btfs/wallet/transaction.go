package wallet

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/beego/beego/v2/core/logs"
	exPb "github.com/tron-us/go-btfs-common/protos/exchange"
	ledgerPb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-btfs-common/utils/grpc"
)

var exchangeService = "https://exchange.bt.co"

var (
	ErrInsufficientExchangeBalanceOnTron   = errors.New("exchange balance on Tron network is not sufficient")
	ErrInsufficientUserBalanceOnTron       = errors.New(fmt.Sprint("User balance on tron network is not sufficient."))
	ErrInsufficientUserBalanceOnLedger     = errors.New("rpc error: code = ResourceExhausted desc = NSF")
	ErrInsufficientExchangeBalanceOnLedger = errors.New("exchange balance on Private Ledger is not sufficient")
)

// Call exchange's Withdraw API
func PrepareWithdraw(ctx context.Context, ledgerAddr, externalAddr []byte, amount, outTxId int64) (
	*exPb.PrepareWithdrawResponse, error) {
	var err error
	var prepareResponse *exPb.PrepareWithdrawResponse
	err = grpc.ExchangeClient(exchangeService).WithContext(ctx,
		func(ctx context.Context, client exPb.ExchangeClient) error {
			prepareWithdrawRequest := &exPb.PrepareWithdrawRequest{
				Amount: amount, OutTxId: outTxId, UserAddress: ledgerAddr, UserExternalAddress: externalAddr}
			prepareResponse, err = client.PrepareWithdraw(ctx, prepareWithdrawRequest)
			if err != nil {
				return err
			}
			logs.Debug(prepareResponse)
			return nil
		})
	if err != nil {
		return nil, err
	}

	return prepareResponse, nil
}

// Call exchange's PrepareWithdraw API
func WithdrawRequest(ctx context.Context, channelId *ledgerPb.ChannelID, ledgerAddr []byte, amount int64,
	prepareResponse *exPb.PrepareWithdrawResponse, privateKey *ecdsa.PrivateKey) (*exPb.WithdrawResponse, error) {
	//make signed success channel state.
	successChannelState := &ledgerPb.ChannelState{
		Id:       channelId,
		Sequence: 1,
		From: &ledgerPb.Account{
			Address: &ledgerPb.PublicKey{
				Key: ledgerAddr,
			},
			Balance: 0,
		},
		To: &ledgerPb.Account{
			Address: &ledgerPb.PublicKey{
				Key: prepareResponse.GetLedgerExchangeAddress(),
			},
			Balance: amount,
		},
	}
	successSignature, err := Sign(successChannelState, privateKey)
	if err != nil {
		return nil, err
	}
	successChannelStateSigned := &ledgerPb.SignedChannelState{Channel: successChannelState, FromSignature: successSignature}

	//make signed fail channel state.
	failChannelState := &ledgerPb.ChannelState{
		Id:       channelId,
		Sequence: 1,
		From: &ledgerPb.Account{
			Address: &ledgerPb.PublicKey{
				Key: ledgerAddr,
			},
			Balance: amount,
		},
		To: &ledgerPb.Account{
			Address: &ledgerPb.PublicKey{
				Key: prepareResponse.GetLedgerExchangeAddress(),
			},
			Balance: 0,
		},
	}
	failSignature, err := Sign(failChannelState, privateKey)
	if err != nil {
		return nil, err
	}

	var withdrawResponse *exPb.WithdrawResponse
	err = grpc.ExchangeClient(exchangeService).WithContext(ctx,
		func(ctx context.Context, client exPb.ExchangeClient) error {
			failChannelStateSigned := &ledgerPb.SignedChannelState{Channel: failChannelState, FromSignature: failSignature}
			//Post the withdraw request.
			withdrawRequest := &exPb.WithdrawRequest{
				Id:                  prepareResponse.GetId(),
				SuccessChannelState: successChannelStateSigned,
				FailureChannelState: failChannelStateSigned,
			}
			withdrawResponse, err = client.Withdraw(ctx, withdrawRequest)
			if err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return withdrawResponse, nil
}
