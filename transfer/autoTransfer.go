package transfer

import (
	"github.com/beego/beego/v2/core/logs"
	"go-torrent-manager/conf"
	model "go-torrent-manager/models"
)

func init() {
	config := conf.Get()

	for _, transferWallet := range config.AutoTransfer {
		go transfer(transferWallet)
	}
}

func transfer(transferWallet model.AutoTransferWallet) {
	logs.Info(transferWallet)
}
