package ipfilter

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var token string
var guid string

func GetWebUiToken() (string, string) {
	if token != "" {
		// todo проверку
	}

	u, err := url.Parse(GetStringEnv("WEB_UI_URL") + ":" + GetStringEnv("WEB_UI_PORT"))
	u.Path = "/gui/token.html"

	client := &http.Client{}
	req, err := http.NewRequest("GET", u.String(), nil)
	req.SetBasicAuth(GetStringEnv("WEB_UI_USERNAME"), GetStringEnv("WEB_UI_PASSWORD"))
	res, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

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

	getTorrentList()

	return token, guid
}

func getTorrentList() string {
	u, err := url.Parse(GetStringEnv("WEB_UI_URL") + ":" + GetStringEnv("WEB_UI_PORT"))
	u.Path = "/gui/"

	q := u.Query()
	q.Add("list", "1")
	q.Add("token", token)
	u.RawQuery = q.Encode()

	client := &http.Client{}
	req, err := http.NewRequest("GET", u.String(), nil)

	req.SetBasicAuth(GetStringEnv("WEB_UI_USERNAME"), GetStringEnv("WEB_UI_PASSWORD"))

	cookie := &http.Cookie{
		Name:  "GUID",
		Value: guid,
		Path:  "/",
	}
	req.AddCookie(cookie)

	res, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	bodyText, err := ioutil.ReadAll(res.Body)
	s := string(bodyText)
	fmt.Println(s)

	var data map[string]interface{}
	json.Unmarshal([]byte(s), &data)
	//data["torrents"]

	//for k, v := range data["torrents"] {
	//	fmt.Println(k, v)
	//}

	fmt.Println(data)

	//tl := make(map[string][]torrentList)
	//json.Unmarshal([]byte(s), &tl)
	//
	//fmt.Printf("\n\n json object:::: %+v", tl)

	return s
}

type torrentList struct {
	Build      int             `json:"build"`
	Torrents   [][]interface{} `json:"torrents"`
	Label      []interface{}   `json:"label"`
	Torrentc   string          `json:"torrentc"`
	Rssfeeds   []interface{}   `json:"rssfeeds"`
	Rssfilters []interface{}   `json:"rssfilters"`
}

// ipfilter

var maxIpFilterLength int
var ipFilterPath string
var ipFilterList = make(map[string]bool)

func IpFilterInit() {
	ipFilterPath = GetIpFilterPath()
	maxIpFilterLength = GetIntEnv("MAX_IP_FILTER_LENGTH")
	fmt.Println("IP FILTER:", ipFilterPath)

	ClearIpFilter()
}

func ClearIpFilter() {
	f, err := os.OpenFile(ipFilterPath, os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Println("CLEARED:", ipFilterPath)
}

func AddToIpFilter(ipList ...string) {
	if len(ipFilterList) > maxIpFilterLength {
		ClearIpFilter()
	}

	count := 0
	var text string
	for _, ip := range ipList {
		if !ipFilterList[ip] {
			ipFilterList[ip] = true
			text += ip + "\n"
			fmt.Println("Block ip:", ip)
			count++
		}
	}

	if count > 0 {
		fmt.Println("Blocked:", count)
		appendToFile(text)
	}
}

func appendToFile(text string) {
	if text == "" {
		return
	}

	f, err := os.OpenFile(ipFilterPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if _, err = f.WriteString(text); err != nil {
		panic(err)
	}
}
