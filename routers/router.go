package routers

import (
	beego "github.com/beego/beego/v2/server/web"
	"go-torrent-manager/controllers"
)

func init() {
	beego.Router("/", &controllers.MainController{})
}
