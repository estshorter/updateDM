package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/sclevine/agouti"
)

const useLINE = true

// Configs defines configuraiton of this app
type Configs struct {
	DriverListURL   string `json:"driverListURL"`
	DriversInfoPath string `json:"driversInfoPath"`
	LINENotifyToken string `json:"lineNotifyToken"`
}

// DriverInfo defines driver infoormation
type DriverInfo struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Notification defines type of a notify function
type Notification func(string) error

func readConfigs(path string) (*Configs, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var configs Configs
	if err := json.Unmarshal(content, &configs); err != nil {
		return nil, err
	}
	return &configs, nil
}

func readDriversInfo(path string) ([]DriverInfo, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var drivers []DriverInfo
	if err := json.Unmarshal(content, &drivers); err != nil {
		return nil, err
	}
	return drivers, nil
}

func writeDriversInfo(path string, drivers []DriverInfo) error {
	jsonText, err := json.MarshalIndent(drivers, "", "\t")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, jsonText, os.ModePerm)
}

func downloadDriverLists(driverListURL string) (io.Reader, error) {
	chromeArgs := agouti.ChromeOptions(
		"args", []string{
			"--headless",
			"--disable-gpu",
		})
	chromeExcludeSwitches := agouti.ChromeOptions(
		"excludeSwitches", []string{
			"enable-logging",
		})

	driver := agouti.ChromeDriver(chromeArgs, chromeExcludeSwitches)
	defer driver.Stop()
	if err := driver.Start(); err != nil {
		return nil, err
	}

	page, err := driver.NewPage()
	if err != nil {
		return nil, err
	}
	fmt.Println("Opening the driver list page...")
	if err := page.Navigate(driverListURL); err != nil {
		return nil, err
	}
	// time.Sleep(time.Millisecond * 500)
	page.FindByID("Download")

	html, err := page.HTML()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader([]byte(html)), nil
}

func scrape(html io.Reader) ([]DriverInfo, error) {
	doc, err := goquery.NewDocumentFromReader(html)
	if err != nil {
		return nil, err
	}
	driverHTML := doc.Find("div#Download > table > tbody > tr")
	if driverHTML.Length() == 0 {
		return nil, errors.New("scraped result is empty")
	}
	drivers := make([]DriverInfo, driverHTML.Length())

	var errInLambda error
	var updatedAt time.Time
	driverHTML.Each(func(idx int, s *goquery.Selection) {
		nameVersion := s.Find("td:first-child").Text()
		posVersionStart := strings.LastIndex(nameVersion, "バージョン:")
		if posVersionStart <= 0 {
			errInLambda = errors.New("version info not found")
			return
		}
		name := nameVersion[0:posVersionStart]
		version := nameVersion[posVersionStart+16:] // add len("バージョン:")
		updatedAt, errInLambda = time.Parse("2006/1/2", s.Find("td:nth-child(4)").Text())
		if err != nil {
			return
		}
		drivers[idx] = DriverInfo{name, version, updatedAt}
	})
	if errInLambda != nil {
		return nil, err
	}
	return drivers, nil
}

func notifyToLINE(msg, token string) error {
	if !useLINE {
		fmt.Println(msg)
		return nil
	}
	values := url.Values{}
	values.Add("message", msg)

	req, err := http.NewRequest(
		"POST",
		"https://notify-api.line.me/api/notify",
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func notifyErrorAndExit(err error, notify Notification) {
	notify(err.Error())
	log.Fatalln(err)
}

// func loadHTMLFromFile(cacheFilePath string) (io.Reader, error) {
// 	content, err := ioutil.ReadFile(cacheFilePath)
// 	return bytes.NewReader(content), err
// }

func contains(target string, list []string) bool {
	for _, r := range list {
		if target == r {
			return true
		}
	}
	return false
}

func checkUpdate(drivers []DriverInfo, driversInfoPath string, notify Notification) error {
	driversExisting, err := readDriversInfo(driversInfoPath)
	if err != nil {
		notify("Created a driver info file as it didn't exist")
		return writeDriversInfo(driversInfoPath, drivers)
	}
	// ディスプレイドライバの名前が2つとも同じなので、そこに変更がある場合はうまく動かない
	if len(drivers) > len(driversExisting) { // 新ドライバの追加
		notify("New driver was added")
		driversExistingName := make([]string, len(driversExisting))
		for i := 0; i < len(driversExisting); i++ {
			driversExistingName[i] = driversExisting[i].Name
		}
		for i := 0; i < len(drivers); i++ {
			if !contains(drivers[i].Name, driversExistingName) {
				notify(drivers[i].Name + " was added")
			}
		}
		return writeDriversInfo(driversInfoPath, drivers)
	}
	if len(drivers) < len(driversExisting) { // 既存ドライバの削除
		notify("Existing driver was removed")
		driversName := make([]string, len(driversExisting))
		for i := 0; i < len(drivers); i++ {
			driversName[i] = drivers[i].Name
		}
		for i := 0; i < len(driversExisting); i++ {
			if !contains(driversExisting[i].Name, driversName) {
				notify(driversExisting[i].Name + " was removed")
			}
		}
		return writeDriversInfo(driversInfoPath, drivers)
	}

	// 名前をハッシュ値にして、順番の変更に対応しようかと思ったが、ディスプレイドライバの名前が2つとも同じなので無理。
	// 順番が変わっていたら、ユーザに通知する仕様にする。
	updated := false
	for i := 0; i < len(drivers); i++ {
		if drivers[i].Name != driversExisting[i].Name { //ドライバの掲載順が変わっていないかチェック
			notify("Listing order has changed. Some drivers may be added or removed. Please check the website.")
			notify("Please delete the drivers info json file manually.")
			return nil // do not overwrite to check which driver got updated
		} else if drivers[i].UpdatedAt.After(driversExisting[i].UpdatedAt) {
			updated = true
			notify(drivers[i].Name + " got a newer version: " + drivers[i].Version + " (updated at " + drivers[i].UpdatedAt.Format("2006/01/02") + ")")
		}
	}
	if updated {
		return writeDriversInfo(driversInfoPath, drivers)
	}
	fmt.Println("No updates available")
	return nil
}

func main() {
	var notify Notification
	var configFilePath string

	flag.Parse()
	if len(flag.Args()) == 1 {
		configFilePath = flag.Args()[0]
	} else {
		configFilePath = "configs.json"
	}
	configs, err := readConfigs(configFilePath)
	if err != nil {
		log.Fatalln(err)
	}

	notify = func(msg string) error { return notifyToLINE(msg, configs.LINENotifyToken) }
	fmt.Println("Downloading html...")
	html, err := downloadDriverLists(configs.DriverListURL)
	if err != nil {
		notifyErrorAndExit(err, notify)
	}
	fmt.Println("Scraping html...")
	drivers, err := scrape(html)
	if err != nil {
		notifyErrorAndExit(err, notify)
	}
	if err := checkUpdate(drivers, configs.DriversInfoPath, notify); err != nil {
		notifyErrorAndExit(err, notify)
	}
}
