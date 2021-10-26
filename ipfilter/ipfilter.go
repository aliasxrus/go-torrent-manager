package ipfilter

import (
	"encoding/json"
	"github.com/beego/beego/v2/core/logs"
	"go-torrent-manager/conf"
	model "go-torrent-manager/models"
	"go-torrent-manager/transfer"
	"golang.org/x/net/html"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var token string
var guid string
var blockedIp = make(map[string]bool)
var errorCounter int64
var errorLimit int64

func Init(wg *sync.WaitGroup) {
	var err error
	config := conf.Get()
	if config.IpFilterConfig.Length == 0 {
		return
	}
	errorLimit = config.IpFilterConfig.ErrorLimit

	if config.IpFilterConfig.StartClient != "" {
		startClient(config.IpFilterConfig)
	}

	for _, transferWallet := range config.AutoTransferWallets {
		if transferWallet.KeyType == "speed" {
			speedTransfer(transferWallet, wg)
			break
		}
	}

	u, err := url.Parse(config.IpFilterConfig.Url + ":" + strconv.Itoa(int(config.IpFilterConfig.Port)))
	if err != nil {
		logs.Error("Ip filter url.", err)
		os.Exit(1)
	}
	u.Path = "/gui/token.html"
	config.IpFilterConfig.GetTokenUrl = u.String()

	if config.IpFilterConfig.Path == "" {
		config.IpFilterConfig.Path = "./ipfilter.dat"
	}

	wg.Add(1)
	go filter(config.IpFilterConfig, wg)
}

func filter(config model.IpFilterConfig, wg *sync.WaitGroup) {
	defer wg.Done()
	ClearIpFilter(config.Path)
	for range time.Tick(time.Duration(config.Interval) * time.Second) {
		if token == "" {
			err := getToken(&config)
			if err != nil {
				token = ""
				errorIncrease()
				continue
			}
			continue
		}

		err := scan(&config)
		if err != nil {
			token = ""
			errorIncrease()
			continue
		}

		err = transfer.SpeedHealthCheck()
		if err != nil {
			errorIncrease()
			continue
		}

		errorCounter = 0
	}
}

func getToken(config *model.IpFilterConfig) error {
	defer func() {
		if r := recover(); r != nil {
			logs.Error("Get token panic")
		}
	}()

	client := &http.Client{}
	req, err := http.NewRequest("GET", config.GetTokenUrl, nil)
	if err != nil {
		logs.Error("Ip filter create get token request.", err)
		return err
	}

	req.SetBasicAuth(config.Username, config.Password)
	res, err := client.Do(req)
	if err != nil {
		logs.Error("Ip filter get token request.", err)
		return err
	}
	defer res.Body.Close()

	cookieGuid := res.Header["Set-Cookie"]
	guid = strings.Split(cookieGuid[0], ";")[0]
	guid = strings.Split(guid, "=")[1]

	z := html.NewTokenizer(res.Body)

	for {
		tt := z.Next()
		if tt == html.TextToken {
			token = z.Token().Data
			break
		}
	}

	return nil
}

func scan(config *model.IpFilterConfig) error {
	defer func() {
		if r := recover(); r != nil {
			logs.Error("Ip filter panic")
		}
	}()

	var err error
	cookie := &http.Cookie{
		Name:  "GUID",
		Value: guid,
		Path:  "/",
	}

	torrentListUrl, err := url.Parse(config.Url + ":" + strconv.Itoa(int(config.Port)))
	if err != nil {
		logs.Error("Ip filter torrent list url.", err)
		return err
	}
	torrentListUrl.Path = "/gui/"

	torrentListQuery := torrentListUrl.Query()
	torrentListQuery.Add("list", "1")
	torrentListQuery.Add("token", token)
	torrentListUrl.RawQuery = torrentListQuery.Encode()

	client := &http.Client{}
	torrentListRequest, err := http.NewRequest("GET", torrentListUrl.String(), nil)
	if err != nil {
		logs.Error("Ip filter create get torrent list request.", err)
		return err
	}
	torrentListRequest.SetBasicAuth(config.Username, config.Password)
	torrentListRequest.AddCookie(cookie)

	torrentListResponse, err := client.Do(torrentListRequest)
	if err != nil {
		logs.Error("Ip filter get torrent list request.", err)
		return err
	}
	defer torrentListResponse.Body.Close()
	torrentListBody, err := ioutil.ReadAll(torrentListResponse.Body)

	var torrentList map[string]interface{}
	err = json.Unmarshal(torrentListBody, &torrentList)
	if err != nil {
		logs.Error("Ip filter unmarshal torrent list.", err)
		return err
	}

	if len(torrentList["torrents"].([]interface{})) == 0 {
		logs.Info("Torrents not found")
		return nil
	}

	torrents := make(map[string]string)
	for _, torrent := range torrentList["torrents"].([]interface{}) {
		torrents[torrent.([]interface{})[0].(string)] = GetTorrentStateInfo(int(torrent.([]interface{})[1].(float64)), int(torrent.([]interface{})[4].(float64)))
	}

	var peerList []interface{}
	torrentsMap := make(map[string]string)
	for key, value := range torrents {
		torrentsMap[key] = value
		if len(torrentsMap) >= 20 {
			peers, err := getPeers(config, cookie, client, torrentsMap)
			if err != nil {
				return err
			}
			peerList = append(peerList, peers...)
			torrentsMap = make(map[string]string)
		}
	}
	if len(torrentsMap) > 0 {
		peers, err := getPeers(config, cookie, client, torrentsMap)
		if err != nil {
			return err
		}
		peerList = append(peerList, peers...)
	}

	var banList []string
	for i := 0; i < len(peerList); i++ {
		state := torrents[peerList[i].(string)]
		i++
		peers := peerList[i].([]interface{})

		for _, peer := range peers {
			ip := peer.([]interface{})[1].(string)
			if blockedIp[ip] {
				continue
			}

			client := peer.([]interface{})[5].(string)
			uploaded := peer.([]interface{})[13].(float64)   // Отдано
			downloaded := peer.([]interface{})[14].(float64) // Загружено
			inactive := peer.([]interface{})[20].(float64)   // Время с последней активности в секундах

			if config.InactiveLimit > 0 && inactive > config.InactiveLimit {
				banList = append(banList, ip)
				continue
			}

			if config.Strategy == 1 && state == "DOWNLOADING" && downloaded > uploaded {
				continue
			}

			if strings.Contains(client, "FAKE") {
				banList = append(banList, ip)
				continue
			}

			isUtVersion := strings.Contains(client, "3.5.5") && !config.ClearUTorrent
			isBtVersion := strings.Contains(client, "7.10.5") && !config.ClearBitTorrent
			isLtVersion := strings.Contains(client, "1.2.2") && !config.ClearLibTorrent
			withBttVersion := isUtVersion || isBtVersion || isLtVersion

			if !withBttVersion {
				banList = append(banList, ip)
				continue
			}
		}
	}
	AddToIpFilter(config, banList)

	return nil
}

func getPeers(config *model.IpFilterConfig, cookie *http.Cookie, client *http.Client, torrents map[string]string) ([]interface{}, error) {
	peerListUrl, err := url.Parse(config.Url + ":" + strconv.Itoa(int(config.Port)))
	if err != nil {
		logs.Error("Ip filter peer list url.", err)
		return nil, err
	}
	peerListUrl.Path = "/gui/"

	peerListQuery := peerListUrl.Query()
	peerListQuery.Add("action", "getpeers")
	peerListQuery.Add("token", token)
	for hash, _ := range torrents {
		peerListQuery.Add("hash", hash)
	}
	peerListUrl.RawQuery = peerListQuery.Encode()

	peerListRequest, err := http.NewRequest("GET", peerListUrl.String(), nil)
	if err != nil {
		logs.Error("Ip filter create get peer list request.", err)
		return nil, err
	}
	peerListRequest.SetBasicAuth(config.Username, config.Password)
	peerListRequest.AddCookie(cookie)

	peerListResponse, err := client.Do(peerListRequest)
	if err != nil {
		logs.Error("Ip filter get peer list request.", err)
		return nil, err
	}
	defer peerListResponse.Body.Close()
	peerListBody, err := ioutil.ReadAll(peerListResponse.Body)

	var peerList map[string]interface{}
	err = json.Unmarshal(peerListBody, &peerList)
	if err != nil {
		logs.Error("Ip filter unmarshal peer list.", err)
		return nil, err
	}

	return peerList["peers"].([]interface{}), err
}

const StateStarted = 1
const StateChecking = 2
const StateError = 16
const StatePaused = 32
const StateQueued = 64

func GetTorrentStateInfo(status int, percentProgress int) string {
	if status&StatePaused != 0 {
		if status&StateChecking != 0 {
			return "CHECKED"
		}
		return "PAUSED"
	} else {
		complete := percentProgress == 1000

		if status&StateStarted != 0 {
			if complete {
				return "SEEDING"
			}
			return "DOWNLOADING"
		} else {
			if status&StateChecking != 0 {
				return "CHECKED"
			} else {
				if status&StateError != 0 {
					return "ERROR"
				} else {
					if status&StateQueued != 0 {
						if complete {
							return "QUEUED_SEED"
						}
						return "QUEUED"
					} else {
						if complete {
							return "FINISHED"
						}
						return "STOPPED"
					}
				}
			}
		}
	}
}

func ClearIpFilter(path string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		logs.Error("Clear ipfilter.dat.", err)
		return
	}
	defer f.Close()
	logs.Info("Ip filter created! Path:", path)
}

func AddToIpFilter(config *model.IpFilterConfig, banList []string) {
	if len(blockedIp) > config.Length {
		blockedIp = make(map[string]bool)
		ClearIpFilter(config.Path)
	}

	count := 0
	var ipListString string
	for _, ip := range banList {
		if !blockedIp[ip] {
			blockedIp[ip] = true
			ipListString += ip + "\n"
			count++
		}
	}

	if count > 0 {
		logs.Debug("Blocked:", count)
		appendToFile(config.Path, ipListString)

		cookie := &http.Cookie{
			Name:  "GUID",
			Value: guid,
			Path:  "/",
		}

		reloadIpFilterUrl, err := url.Parse(config.Url + ":" + strconv.Itoa(int(config.Port)))
		if err != nil {
			logs.Error("Reload ip filter.", err)
			return
		}
		reloadIpFilterUrl.Path = "/gui/"

		reloadIpFilterQuery := reloadIpFilterUrl.Query()
		reloadIpFilterQuery.Add("action", "setsetting")
		reloadIpFilterQuery.Add("s", "ipfilter.enable")
		reloadIpFilterQuery.Add("v", "1")
		reloadIpFilterQuery.Add("token", token)
		reloadIpFilterUrl.RawQuery = reloadIpFilterQuery.Encode()

		client := &http.Client{}
		torrentListRequest, err := http.NewRequest("GET", reloadIpFilterUrl.String(), nil)
		if err != nil {
			logs.Error("Reload ip filter create request.", err)
			return
		}
		torrentListRequest.SetBasicAuth(config.Username, config.Password)
		torrentListRequest.AddCookie(cookie)

		torrentListResponse, err := client.Do(torrentListRequest)
		if err != nil {
			logs.Error("Reload ip filter request.", err)
			return
		}
		defer torrentListResponse.Body.Close()
	}
}

func appendToFile(path string, text string) {
	if text == "" {
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		logs.Error("Open ipfilter.dat.", err)
		return
	}
	defer f.Close()

	if _, err = f.WriteString(text); err != nil {
		logs.Error("Write to ipfilter.dat.", err)
		return
	}
}

func errorIncrease() {
	errorCounter++

	if errorLimit > 0 && errorCounter > errorLimit {
		logs.Error("Error limit.", errorCounter)
		os.Exit(1)
	}
}

func startClient(config model.IpFilterConfig) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(config.StartClient)
	} else {
		cmd = exec.Command("xvfb-run", "wine", config.StartClient)
		cmd.Env = append(os.Environ(), "LANG=C.UTF-8")
	}

	err := cmd.Start()
	if err != nil {
		logs.Error("Start client.", err)
		os.Exit(1)
	}
	logs.Info("Client started...")
}

func speedTransfer(transferWallet model.AutoTransferWallet, wg *sync.WaitGroup) {
	var err error
	for i := 20; i >= 0; i-- {
		transferWallet.KeyValue, err = transfer.GetSpeedKey(transferWallet)
		if err != nil {
			logs.Error("Get speed key for transfer.", err)
			if i == 0 {
				os.Exit(1)
			}
			errorIncrease()
			time.Sleep(20 * time.Second)
			continue
		}
		break
	}

	transferWallet.KeyType = "key"
	if transferWallet.Interval < 1 {
		transferWallet.Interval = 1
	}
	wg.Add(1)
	go transfer.Transfer(transferWallet, wg)
}
