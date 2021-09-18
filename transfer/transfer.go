package transfer

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/beego/beego/v2/core/logs"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"
	"go-torrent-manager/btfs/util"
	"go-torrent-manager/btfs/wallet"
	"go-torrent-manager/conf"
	model "go-torrent-manager/models"
	"math"
	"os"
	"time"
)

var escrowService = "https://escrow.btfs.io"

func init() {
	config := conf.Get()

	for _, transferWallet := range config.AutoTransferWallets {
		if transferWallet.Interval < 1 {
			transferWallet.Interval = 1
		}
		go transfer(transferWallet)
	}
}

func transfer(transferWallet model.AutoTransferWallet) {
	var balance model.Balance
	address, err := util.GetAddress(transferWallet.KeyType, transferWallet.KeyValue)
	if err != nil {
		logs.Error("Generate key for transfer.", err)
		os.Exit(1)
	}
	logs.Info("Transfer added:", address.Base58Address, ", Interval:", transferWallet.Interval, ", Recipient:", transferWallet.Recipient)

	for true {
		time.Sleep(time.Duration(transferWallet.Interval) * time.Second)
		balance.LedgerBalance, err = wallet.GetLedgerBalance(address)
		if err != nil {
			logs.Error("Wallet:", transferWallet.Name, "Get balance error.", err.Error())
			continue
		}
		logs.Info("Wallet:", transferWallet.Name, "Balance:", math.Floor(float64(balance.LedgerBalance))/1000000, "Sum:", math.Floor(float64(transferWallet.Sum))/1000000)
		if balance.LedgerBalance == 0 {
			continue
		}

		recipient, err := base64.StdEncoding.DecodeString(transferWallet.Recipient)
		if err != nil {
			logs.Error("Wallet:", transferWallet.Name, "Decode recipient base64 error.", err.Error())
			continue
		}

		transferRequest := &ledgerpb.TransferRequest{
			Payer:     &ledgerpb.PublicKey{Key: address.LedgerAddress},
			Recipient: &ledgerpb.PublicKey{Key: recipient},
			Amount:    balance.LedgerBalance,
		}

		raw, err := proto.Marshal(transferRequest)
		if err != nil {
			logs.Error("Wallet:", transferWallet.Name, "Get raw error.", err.Error())
			continue
		}

		signature, err := wallet.SignChannel(raw, address.PrivateKeyEcdsa)
		if err != nil {
			logs.Error("Wallet:", transferWallet.Name, "Sign channel error.", err.Error())
			continue
		}

		request := &ledgerpb.SignedTransferRequest{
			TransferRequest: transferRequest,
			Signature:       signature,
		}

		err = grpc.EscrowClient(escrowService).WithContext(context.Background(),
			func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
				response, err := client.Pay(ctx, request)
				if err != nil {
					return err
				}
				if response == nil {
					return fmt.Errorf("escrow reponse is nil")
				}
				transferWallet.Sum += request.TransferRequest.Amount
				logs.Info("Wallet:", transferWallet.Name, "Balance after:", math.Floor(float64(response.Balance))/1000000, "Sum:", math.Floor(float64(transferWallet.Sum))/1000000)
				return nil
			})
		if err != nil {
			logs.Error("Wallet:", transferWallet.Name, "Transfer request error.", err.Error())
		}
	}
}
