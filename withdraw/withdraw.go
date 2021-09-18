package withdraw

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/beego/beego/v2/core/logs"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	exPb "github.com/tron-us/go-btfs-common/protos/exchange"
	ledgerPb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"go-torrent-manager/btfs/util"
	"go-torrent-manager/btfs/wallet"
	"go-torrent-manager/conf"
	model "go-torrent-manager/models"
	"net/http"
	"os"
	"strconv"
	"time"
)

var escrowService = "https://escrow.btfs.io"

var (
	ErrInsufficientUserBalanceOnLedger = errors.New("rpc error: code = ResourceExhausted desc = NSF")
)

func init() {
	var err error
	config := conf.Get()

	if config.AutoWithdrawWallets == nil {
		return
	}

	if config.AutoWithdrawConfig.Interval < 100 {
		config.AutoWithdrawConfig.Interval = 100
	}

	for i, withdrawWallet := range config.AutoWithdrawWallets {
		config.AutoWithdrawWallets[i].Address, err = util.GetAddress(withdrawWallet.KeyType, withdrawWallet.KeyValue)
		if err != nil {
			logs.Error("Generate key for withdraw.", err)
			os.Exit(1)
		}
	}

	go autoWithdraw(&config.AutoWithdrawWallets, &config)
}

func autoWithdraw(wallets *[]model.AutoWithdrawWallet, config *model.Config) {
	logs.Info("Start auto withdraw")
	refreshBalances(*wallets)
	for count := int64(0); true; count++ {
		time.Sleep(time.Duration(config.AutoWithdrawConfig.Interval) * time.Millisecond)

		gatewayBalance := getGatewayBalance(config)
		logs.Debug("Gateway balance:", gatewayBalance.BttBalance/1000000)
		if gatewayBalance.BttBalance < 1000000000 ||
			gatewayBalance.TrxBalance < 282000 {
			continue
		}

		for _, withdrawWallet := range config.AutoWithdrawWallets {
			if gatewayBalance.BttBalance < withdrawWallet.MinAmount*1000000 ||
				withdrawWallet.LedgerBalance < 1000000000 ||
				(withdrawWallet.Difference > 0 && withdrawWallet.GatewayBalance.BttBalance-gatewayBalance.BttBalance < withdrawWallet.Difference) {
				continue
			}

			withdraw(&withdrawWallet, &gatewayBalance)
			withdrawWallet.GatewayBalance = gatewayBalance
		}

		if count%config.AutoWithdrawConfig.Refresh == 0 {
			refreshBalances(*wallets)
		}
	}
}

func withdraw(withdrawWallet *model.AutoWithdrawWallet, gatewayBalance *model.Balance) {
	amount := withdrawWallet.LedgerBalance
	if amount > 99999000000 {
		amount = 99999000000
	}

	if gatewayBalance.BttBalance > amount {
		amount = gatewayBalance.BttBalance
	}

	go sendWithdraw(withdrawWallet, amount)
	withdrawWallet.LedgerBalance -= amount
}

func sendWithdraw(withdrawWallet *model.AutoWithdrawWallet, amount int64) {
	logs.Info("Withdraw begin!", withdrawWallet.Name)
	outTxId := time.Now().UnixNano()

	//PrepareWithdraw
	prepareResponse, err := wallet.PrepareWithdraw(context.Background(), withdrawWallet.Address.LedgerAddress, withdrawWallet.Address.TronAddress, amount, outTxId)
	if err != nil {
		logs.Error("Send withdraw, PrepareWithdraw", err.Error())
		return
	}
	if prepareResponse.Response.Code != exPb.Response_SUCCESS {
		logs.Error("Send withdraw, PrepareWithdraw, response code:", prepareResponse.Response.Code, string(prepareResponse.Response.ReturnMessage))
		return
	}
	logs.Debug("Prepare withdraw success, id:", prepareResponse.GetId())

	channelCommit := &ledgerPb.ChannelCommit{
		Payer:     &ledgerPb.PublicKey{Key: withdrawWallet.Address.LedgerAddress},
		Recipient: &ledgerPb.PublicKey{Key: prepareResponse.GetLedgerExchangeAddress()},
		Amount:    amount,
		PayerId:   time.Now().UnixNano() + prepareResponse.GetId(),
	}
	//Sign channel commit.
	signature, err := wallet.Sign(channelCommit, withdrawWallet.Address.PrivateKeyEcdsa)
	if err != nil {
		logs.Error("Send withdraw, signature", string(prepareResponse.Response.ReturnMessage))
		return
	}

	var channelId *ledgerPb.ChannelID
	err = grpc.EscrowClient(escrowService).WithContext(context.Background(),
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			channelId, err = client.CreateChannel(ctx,
				&ledgerPb.SignedChannelCommit{Channel: channelCommit, Signature: signature})
			if err != nil {
				if err.Error() == ErrInsufficientUserBalanceOnLedger.Error() {
					return ErrInsufficientUserBalanceOnLedger
				}
				return err
			}
			return nil
		})
	if err != nil {
		logs.Error("Send withdraw, CreateChannel", string(prepareResponse.Response.ReturnMessage))
		return
	}
	logs.Debug("CreateChannel success, channelId:", channelId.GetId())

	//Do the WithdrawRequest.
	withdrawResponse, err := wallet.WithdrawRequest(context.Background(), channelId, withdrawWallet.Address.LedgerAddress, amount, prepareResponse, withdrawWallet.Address.PrivateKeyEcdsa)
	logs.Info("withdrawResponse:", string(withdrawResponse.Response.ReturnMessage))
	if err != nil {
		logs.Error("Send withdraw, WithdrawRequest", err.Error())
		return
	}

	if withdrawResponse.Response.Code != exPb.Response_SUCCESS {
		logs.Error("Send withdraw, withdrawResponse", string(withdrawResponse.Response.ReturnMessage))
		return
	}
	logs.Debug("Withdraw end!", withdrawWallet.Name)
	logs.Info("Withdraw submitted!", channelId.Id, prepareResponse.GetId())
}

func getGatewayBalance(config *model.Config) model.Balance {
	var gateway model.TronScanResponse
	var balance model.Balance
	r, err := http.Get(config.AutoWithdrawConfig.Url)
	if err != nil {
		logs.Error("Get gateway balance error.", err)
		return balance
	}
	defer r.Body.Close()

	err = json.NewDecoder(r.Body).Decode(&gateway)
	if err != nil {
		logs.Error("Parse json gateway balance error.", err)
		return balance
	}

	if gateway.Data == nil {
		balance.FreeNetUsage = gateway.Bandwidth.FreeNetUsed
		for _, tokenBalances := range gateway.TokenBalances {
			if tokenBalances.TokenId == "_" {
				TrxBalance, err := strconv.Atoi(tokenBalances.Balance)
				if err != nil {
					logs.Error("Parse TrxBalance error.", err)
					return balance
				}
				balance.TrxBalance = int64(TrxBalance)
			}

			if tokenBalances.TokenId == "1002000" {
				BttBalance, err := strconv.Atoi(tokenBalances.Balance)
				if err != nil {
					logs.Error("Parse BttBalance error.", err)
					return balance
				}
				balance.BttBalance = int64(BttBalance)
			}
		}
	} else {
	}

	return balance
}

func refreshBalances(withdrawWallets []model.AutoWithdrawWallet) {
	for i, withdrawWallet := range withdrawWallets {
		ledgerBalance, err := wallet.GetLedgerBalance(withdrawWallet.Address)
		if err != nil {
			logs.Error("Wallet:", withdrawWallet.Name, "Get balance error.", err.Error())
			continue
		}
		withdrawWallets[i].LedgerBalance = ledgerBalance
		logs.Info("Wallet", withdrawWallets[i].Name, ", ledger balance:", ledgerBalance/1000000)
	}
}

//hex.EncodeToString(addr.Bytes())
