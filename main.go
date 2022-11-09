package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	BiosListURL     string `json:"biosListURL"`
	BiosInfoPath    string `json:"biosInfoPath"`
	LINENotifyToken string `json:"lineNotifyToken"`
	OS              string `json:"OS"`
}

// DriverInfo defines driver infoormation
type DriverInfo struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type BiosInfo struct {
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Notification defines type of a notify function
type Notification func(string) error

func readConfigs(path string) (*Configs, error) {
	content, err := os.ReadFile(path)
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
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var drivers []DriverInfo
	if err := json.Unmarshal(content, &drivers); err != nil {
		return nil, err
	}
	return drivers, nil
}

func readBiosInfo(path string) ([]BiosInfo, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var biosList []BiosInfo
	if err := json.Unmarshal(content, &biosList); err != nil {
		return nil, err
	}
	return biosList, nil
}

func writeDriversInfo(path string, drivers []DriverInfo) error {
	jsonText, err := json.MarshalIndent(drivers, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, jsonText, os.ModePerm)
}

func writeBiosInfo(path string, drivers []BiosInfo) error {
	jsonText, err := json.MarshalIndent(drivers, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, jsonText, os.ModePerm)
}

func downloadPage(driverListURL string) (io.Reader, error) {
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
	// fmt.Println("Opening the page...")
	if err := page.Navigate(driverListURL); err != nil {
		return nil, err
	}
	// time.Sleep(time.Millisecond * 500)

	html, err := page.HTML()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader([]byte(html)), nil
}

func downloadDriverLists(driverListURL string) (io.Reader, error) {
	return downloadPage(driverListURL)
}

func downloadBiosLists(driverListURL string) (io.Reader, error) {
	return downloadPage(driverListURL)
}

func scrapeDriverList(html io.Reader, OS string) ([]DriverInfo, error) {
	doc, err := goquery.NewDocumentFromReader(html)
	if err != nil {
		return nil, err
	}
	// fmt.Println(doc.Text())
	driverHTML := doc.Find("div#Download > table > tbody > tr")
	if driverHTML.Length() == 0 {
		return nil, errors.New("scraped result is empty")
	}
	drivers := []DriverInfo{}

	var errInLambda error
	var updatedAt time.Time
	driverHTML.Each(func(idx int, s *goquery.Selection) {
		if s.Find("td:nth-child(2)").Text() != OS {
			return
		}
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
		drivers = append(drivers, DriverInfo{name, version, updatedAt})
	})
	if errInLambda != nil {
		return nil, err
	}
	return drivers, nil
}

func scrapeBiosList(html io.Reader) ([]BiosInfo, error) {
	doc, err := goquery.NewDocumentFromReader(html)
	if err != nil {
		return nil, err
	}
	driverHTML := doc.Find("div#BIOS > table > tbody > tr")
	if driverHTML.Length() == 0 {
		return nil, errors.New("scraped result is empty")
	}
	biosList := make([]BiosInfo, driverHTML.Length())

	var errInLambda error
	var updatedAt time.Time
	driverHTML.Each(func(idx int, s *goquery.Selection) {
		version := s.Find("td:first-child").Text()
		updatedAt, errInLambda = time.Parse("2006/1/2", s.Find("td:nth-child(2)").Text())
		if err != nil {
			return
		}
		biosList[idx] = BiosInfo{version, updatedAt}
	})
	if errInLambda != nil {
		return nil, err
	}
	return biosList, nil
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
// 	content, err := os.ReadFile(cacheFilePath)
// 	return bytes.NewReader(content), err
// }

// func contains(target string, list []string) bool {
// 	for _, r := range list {
// 		if target == r {
// 			return true
// 		}
// 	}
// 	return false
// }

func checkDriverUpdate(drivers []DriverInfo, driversInfoPath, url string, notify Notification) error {
	driversExisting, err := readDriversInfo(driversInfoPath)
	if err != nil {
		notify("Created a driver info file as it didn't exist")
		return writeDriversInfo(driversInfoPath, drivers)
	}
	// ディスプレイドライバの名前が2つとも同じなので、そこに変更がある場合はうまく動かない
	if len(drivers) > len(driversExisting) { // 新ドライバの追加
		notify("New driver was added")
		existingNameSet := make(map[string]struct{}, len(driversExisting))
		for i := 0; i < len(driversExisting); i++ {
			existingNameSet[driversExisting[i].Name] = struct{}{}
		}
		for i := 0; i < len(drivers); i++ {
			if _, ok := existingNameSet[drivers[i].Name]; !ok {
				notify(drivers[i].Name + " was added")
			}
		}
		notify(url)
		return writeDriversInfo(driversInfoPath, drivers)
	}
	if len(drivers) < len(driversExisting) { // 既存ドライバの削除
		notify("Existing driver was removed")
		nameSet := make(map[string]struct{}, len(drivers))
		for i := 0; i < len(drivers); i++ {
			nameSet[drivers[i].Name] = struct{}{}
		}
		for i := 0; i < len(driversExisting); i++ {
			if _, ok := nameSet[driversExisting[i].Name]; !ok {
				notify(driversExisting[i].Name + " was removed")
			}
		}
		notify(url)
		return writeDriversInfo(driversInfoPath, drivers)
	}

	// 名前をハッシュ値にして、順番の変更に対応しようかと思ったが、ディスプレイドライバの名前が2つとも同じなので無理。
	// 順番が変わっていたら、ユーザに通知する仕様にする。
	updated := false
	for i := 0; i < len(drivers); i++ {
		if drivers[i].Name != driversExisting[i].Name { //ドライバの掲載順が変わっていないかチェック
			notify("[Driver] Version mismatch detected: existing: " + driversExisting[i].Version + ", got: " + drivers[i].Version)
			notify("[Driver] Listing order has changed. Some drivers may be added or removed. Please check the website.")
			notify("Please delete the drivers info json file manually.")
			notify(url)
			return nil // do not overwrite to check which driver got updated
		} else if drivers[i].UpdatedAt.After(driversExisting[i].UpdatedAt) {
			updated = true
			notify(drivers[i].Name + " got a newer version: " + drivers[i].Version + " (updated at " + drivers[i].UpdatedAt.Format("2006/01/02") + ")")
		}
	}
	if updated {
		notify(url)
		return writeDriversInfo(driversInfoPath, drivers)
	}
	fmt.Println("No updates available")
	return nil
}

func checkBiosUpdate(biosList []BiosInfo, biosInfoPath, url string, notify Notification) error {
	existing, err := readBiosInfo(biosInfoPath)
	if err != nil {
		notify("Created a bios info file as it didn't exist")
		return writeBiosInfo(biosInfoPath, biosList)
	}

	if len(biosList) > len(existing) { // 新BIOSの追加
		notify("New driver was added")
		biosExistingVersionSet := make(map[string]struct{}, len(existing))
		for i := 0; i < len(existing); i++ {
			biosExistingVersionSet[existing[i].Version] = struct{}{}
		}
		for i := 0; i < len(biosList); i++ {
			if _, ok := biosExistingVersionSet[biosList[i].Version]; !ok {
				notify("BIOS " + biosList[i].Version + " was added")
			}
		}
		notify(url)
		return writeBiosInfo(biosInfoPath, biosList)
	}
	if len(biosList) < len(existing) { // 既存BIOSの削除
		notify("Existing driver was removed")
		biosVersionsSet := make(map[string]struct{}, len(existing))
		for i := 0; i < len(biosList); i++ {
			biosVersionsSet[biosList[i].Version] = struct{}{}
		}
		for i := 0; i < len(existing); i++ {
			if _, ok := biosVersionsSet[existing[i].Version]; !ok {
				notify("BIOS " + existing[i].Version + " was removed")
			}
		}
		notify(url)
		return writeBiosInfo(biosInfoPath, biosList)
	}

	// 名前をハッシュ値にして、順番の変更に対応しようかと思ったが、ディスプレイドライバの名前が2つとも同じなので無理。
	// 順番が変わっていたら、ユーザに通知する仕様にする。
	updated := false
	for i := 0; i < len(biosList); i++ {
		if biosList[i].Version != existing[i].Version { //ドライバの掲載順が変わっていないかチェック
			notify("[Bios] Version mismatch detected: existing: " + existing[i].Version + ", got: " + biosList[i].Version)
			notify("[Bios] Listing order has changed. Some bioses may be added or removed. Please check the website.")
			notify("Please delete the bios info json file manually.")
			notify(url)
			return nil // do not overwrite to check which driver got updated
		} else if biosList[i].UpdatedAt.After(existing[i].UpdatedAt) {
			updated = true
			notify("Bios got a newer version: " + biosList[i].Version + " (updated at " + biosList[i].UpdatedAt.Format("2006/01/02") + ")")
		}
	}
	if updated {
		notify(url)
		return writeBiosInfo(biosInfoPath, biosList)
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
	html_drivers, err := downloadDriverLists(configs.DriverListURL)
	if err != nil {
		notifyErrorAndExit(err, notify)
	}
	fmt.Println("Downloaded driver page")
	html_bios, err := downloadBiosLists(configs.BiosListURL)
	if err != nil {
		notifyErrorAndExit(err, notify)
	}
	fmt.Println("Downloaded BIOS page")
	fmt.Println("Scraping html...")
	drivers, err := scrapeDriverList(html_drivers, configs.OS)
	if err != nil {
		notifyErrorAndExit(err, notify)
	}
	if err := checkDriverUpdate(drivers, configs.DriversInfoPath, configs.DriverListURL, notify); err != nil {
		notifyErrorAndExit(err, notify)
	}

	bioses, err := scrapeBiosList(html_bios)
	if err != nil {
		notifyErrorAndExit(err, notify)
	}
	if err := checkBiosUpdate(bioses, configs.BiosInfoPath, configs.BiosListURL, notify); err != nil {
		notifyErrorAndExit(err, notify)
	}
}
