# updateDM

Check updates of Asrock DeskMini X300 driver and BIOS by scraping https://www.asrock.com/nettop/AMD/DeskMini%20X300%20Series/index.jp.asp#Download.

## Usage
Place the following `configs.json` in the same folder as `updateDM.exe`.
```
{	
	"driverListURL": "https://www.asrock.com/nettop/AMD/DeskMini%20X300%20Series/index.jp.asp#Download",
	"driversInfoPath": "drivers_info.json",
	"biosListURL":  "https://www.asrock.com/nettop/AMD/DeskMini%20X300%20Series/index.jp.asp#BIOS",
	"biosInfoPath": "bios_info.json",
	"lineNotifyToken": "YOUR_LINE_TOKEN",
	"OS": "Windows® 10 64bit"
}
```

Location of `configs.json` can be passed to the program through a command line argument.

LINE token can be obtained from https://notify-bot.line.me/ja/.
