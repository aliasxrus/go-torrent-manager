package main

import (
	"github.com/beego/beego/v2/core/logs"
	beego "github.com/beego/beego/v2/server/web"
	"go-torrent-manager/conf"
	_ "go-torrent-manager/routers"
	_ "go-torrent-manager/transfer"
)

func main() {
	config := conf.Get()
	logs.Info("Version:", config.Version)
	beego.Run()
}
