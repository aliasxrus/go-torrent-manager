package transfer

import (
	"github.com/beego/beego/v2/core/logs"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"go-torrent-manager/btfs/util"
	"go-torrent-manager/conf"
	"time"

	"context"
	model "go-torrent-manager/models"
	"os"
)

var escrowService = "https://escrow.btfs.io"
var solidityService = "grpc.trongrid.io:50052"

func init() {
	config := conf.Get()

	for _, transferWallet := range config.AutoTransfer {
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
		balance.LedgerBalance, err = GetLedgerBalance(address)

		logs.Info("Wallet:", transferWallet.Name, "Balance:", balance.LedgerBalance)
	}
}

func GetLedgerBalance(address model.Address) (int64, error) {
	privKey, err := address.Identity.DecodePrivateKey("")
	if err != nil {
		return 0, err
	}
	lgSignedPubKey, err := ledger.NewSignedPublicKey(privKey, privKey.GetPublic())

	var balance int64 = 0
	err = grpc.EscrowClient(escrowService).WithContext(context.Background(),
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.BalanceOf(ctx, ledger.NewSignedCreateAccountRequest(lgSignedPubKey.Key, lgSignedPubKey.Signature))
			if err != nil {
				return err
			}
			balance = res.Result.Balance
			return nil
		})
	if err != nil {
		return 0, err
	}

	return balance, nil
}
