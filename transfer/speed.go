package transfer

import (
	"bytes"
	"errors"
	"fmt"
	model "go-torrent-manager/models"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tron-us/go-btfs-common/crypto"
)

const (
	api            = "http://127.0.0.1:%d/api"
	setPasswordUrl = api + "/password?t=%s"
	keyUrl         = api + "/private_key?pw=%s&t=%s"
	tokenUrl       = api + "/token"
)

var speedPort int64

func GetSpeedKey(wallet model.AutoTransferWallet) (string, error) {
	password := url.QueryEscape(wallet.SpeedPassword)
	pf, err := os.Open(wallet.PortFile)
	if err != nil {
		return "", err
	}
	port, err := readPort(pf)
	if err != nil {
		return "", err
	}
	speedPort = port
	token, err := get(fmt.Sprintf(tokenUrl, port))
	if err != nil {
		return "", err
	}
	err = setPassword(fmt.Sprintf(setPasswordUrl, port, token), password)
	if err != nil {
		return "", err
	}
	time.Sleep(3 * time.Second)
	key, err := get(fmt.Sprintf(keyUrl, port, password, token))
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", errors.New("invalid private key")
	}
	base64, err := crypto.Hex64ToBase64(key)
	if err != nil {
		return "", err
	}

	return base64, nil
}

func readPort(r io.Reader) (int64, error) {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return -1, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(bytes)), 10, 32)
}

func get(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func setPassword(url string, password string) error {
	r := bytes.NewReader([]byte(password))
	resp, err := http.Post(url, "text/plain", r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func SpeedHealthCheck() error {
	_, err := get(fmt.Sprintf(tokenUrl, speedPort))
	if err != nil {
		return err
	}
	return nil
}
