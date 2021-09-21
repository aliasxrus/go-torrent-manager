package withdraw

import (
	"context"
	"encoding/base64"
	"encoding/hex"
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
var ErrInsufficientUserBalanceOnLedger = errors.New("rpc error: code = ResourceExhausted desc = NSF")
var balanceChannel = make(chan model.BalanceChannel, 10)

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
		if withdrawWallet.BttRecipientAddress == "" {
			logs.Info(config.AutoWithdrawWallets[i].Name, "BttRecipientAddress:", hex.EncodeToString(config.AutoWithdrawWallets[i].Address.TronAddress))
		} else {
			logs.Info(config.AutoWithdrawWallets[i].Name, "BttRecipientAddress:", withdrawWallet.BttRecipientAddress)
		}
		logs.Info(config.AutoWithdrawWallets[i].Name, "LedgerAddress:", base64.StdEncoding.EncodeToString(config.AutoWithdrawWallets[i].Address.LedgerAddress))
	}

	go refreshBalances(&config)
	go autoWithdraw(&config)
}

func autoWithdraw(config *model.Config) {
	logs.Info("Start auto withdraw")

	previousGatewayBalance := model.Balance{BttBalance: -1}
	for count := int64(0); true; count++ {
		for len(balanceChannel) > 0 {
			balance := <-balanceChannel
			if config.AutoWithdrawWallets[balance.WalletIndex].LedgerBalance != balance.LedgerBalance {
				config.AutoWithdrawWallets[balance.WalletIndex].LedgerBalance = balance.LedgerBalance
				logs.Info("Wallet", config.AutoWithdrawWallets[balance.WalletIndex].Name, ", ledger balance:", balance.LedgerBalance/1000000)
			}
		}

		time.Sleep(time.Duration(config.AutoWithdrawConfig.Interval) * time.Millisecond)
		// Минимальный таймаут между попытками вывода, в миллисекундах
		if config.AutoWithdrawConfig.TimeoutWithdraw > time.Since(config.AutoWithdrawConfig.LastWithdraw).Milliseconds() {
			continue
		}

		gatewayBalance := getGatewayBalance(config)
		if previousGatewayBalance.BttBalance != gatewayBalance.BttBalance {
			logs.Info("Gateway balance:", gatewayBalance.BttBalance/1000000)
			previousGatewayBalance = gatewayBalance
		}
		previousGatewayBalance = gatewayBalance

		if gatewayBalance.BttBalance < 1000000000 || gatewayBalance.TrxBalance < 282000 {
			continue
		}

		for i, withdrawWallet := range config.AutoWithdrawWallets {
			if gatewayBalance.BttBalance < withdrawWallet.MinAmount*1000000 || // Минимальный баланс на шлюзе
				(withdrawWallet.MaxAmount > 0 && gatewayBalance.BttBalance > withdrawWallet.MaxAmount*1000000) || // Максимальный баланс на шлюзе
				withdrawWallet.LedgerBalance < 1000000000 || // Недостаточно средств для вывода
				(withdrawWallet.Difference > 0 && withdrawWallet.GatewayBalance.BttBalance-gatewayBalance.BttBalance < withdrawWallet.Difference) || // Разница в балансе
				withdrawWallet.TimeoutWalletWithdraw > time.Since(withdrawWallet.LastWalletWithdraw).Milliseconds() || // Таймаут по выводам с одного кошелька
				config.AutoWithdrawConfig.TimeoutWithdraw > time.Since(config.AutoWithdrawConfig.LastWithdraw).Milliseconds() { // Таймаут по выводам
				config.AutoWithdrawWallets[i].GatewayBalance = gatewayBalance
				continue
			}

			amount := withdrawWallet.LedgerBalance
			if amount > 99999000000 {
				amount = 99999000000
			}
			if amount > gatewayBalance.BttBalance {
				amount = gatewayBalance.BttBalance
			}

			go withdraw(withdrawWallet, amount)
			config.AutoWithdrawWallets[i].LastWalletWithdraw = time.Now()
			config.AutoWithdrawConfig.LastWithdraw = time.Now()
			config.AutoWithdrawWallets[i].GatewayBalance = gatewayBalance
			config.AutoWithdrawWallets[i].LedgerBalance -= amount
		}
	}
}

func withdraw(withdrawWallet model.AutoWithdrawWallet, amount int64) {
	logs.Info("Withdraw begin!", withdrawWallet.Name, "Amount:", amount)
	outTxId := time.Now().UnixNano() + time.Now().UnixNano()

	if withdrawWallet.BttRecipientAddress != "" {
		decodeString, err := hex.DecodeString(withdrawWallet.BttRecipientAddress)
		if err != nil {
			logs.Error("Send withdraw, decodeString BttRecipientAddress", withdrawWallet.Name, err.Error())
			return
		}
		withdrawWallet.Address.TronAddress = decodeString
	}
	//PrepareWithdraw
	prepareResponse, err := wallet.PrepareWithdraw(context.Background(), withdrawWallet.Address.LedgerAddress, withdrawWallet.Address.TronAddress, amount, outTxId)
	if err != nil {
		logs.Error("Send withdraw, PrepareWithdraw", withdrawWallet.Name, err.Error())
		return
	}
	if prepareResponse.Response.Code != exPb.Response_SUCCESS {
		logs.Error("Send withdraw, PrepareWithdraw, response code", withdrawWallet.Name, prepareResponse.Response.Code, string(prepareResponse.Response.ReturnMessage))
		return
	}
	logs.Debug("Prepare withdraw success, id", withdrawWallet.Name, prepareResponse.GetId())

	channelCommit := &ledgerPb.ChannelCommit{
		Payer:     &ledgerPb.PublicKey{Key: withdrawWallet.Address.LedgerAddress},
		Recipient: &ledgerPb.PublicKey{Key: prepareResponse.GetLedgerExchangeAddress()},
		Amount:    amount,
		PayerId:   time.Now().UnixNano() + prepareResponse.GetId(),
	}
	//Sign channel commit.
	signature, err := wallet.Sign(channelCommit, withdrawWallet.Address.PrivateKeyEcdsa)
	if err != nil {
		logs.Error("Send withdraw, signature", withdrawWallet.Name, string(prepareResponse.Response.ReturnMessage))
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
		logs.Error("Send withdraw, CreateChannel", withdrawWallet.Name, string(prepareResponse.Response.ReturnMessage))
		return
	}
	logs.Debug("CreateChannel success, channelId:", withdrawWallet.Name, channelId.GetId())

	//Do the WithdrawRequest.
	withdrawResponse, err := wallet.WithdrawRequest(context.Background(), channelId, withdrawWallet.Address.LedgerAddress, amount, prepareResponse, withdrawWallet.Address.PrivateKeyEcdsa)
	logs.Info("withdrawResponse:", withdrawWallet.Name, string(withdrawResponse.Response.ReturnMessage))
	if err != nil {
		logs.Error("Send withdraw, WithdrawRequest", withdrawWallet.Name, err.Error())
		return
	}

	if withdrawResponse.Response.Code != exPb.Response_SUCCESS {
		logs.Error("Send withdraw, withdrawResponse", withdrawWallet.Name, string(withdrawResponse.Response.ReturnMessage))
		return
	}
	logs.Info("CONGRATULATION! Withdraw submitted!", withdrawWallet.Name, channelId.Id, prepareResponse.GetId())
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

func refreshBalances(config *model.Config) {
	for true {
		for i, withdrawWallet := range config.AutoWithdrawWallets {
			ledgerBalance, err := wallet.GetLedgerBalance(withdrawWallet.Address)
			if err != nil {
				logs.Error("Wallet:", withdrawWallet.Name, "Get balance error.", err.Error())
				continue
			}
			balanceChannel <- model.BalanceChannel{WalletIndex: i, LedgerBalance: ledgerBalance}
		}
		time.Sleep(time.Duration(config.AutoWithdrawConfig.RefreshTimeout) * time.Second)
	}
}

//hex.EncodeToString(addr.Bytes())
