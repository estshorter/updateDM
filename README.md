updateDM

Check updates of Asrock DeskMini X300 driver

## Usage
Place the following `configs.json` in the same folder as `updateDM.exe`.
```
{	
	"DriverListURL": "https://www.asrock.com/nettop/AMD/DeskMini%20X300%20Series/index.jp.asp#Download",
	"driversInfoPath": "drivers_info.json",
	"lineNotifyToken": "YOUR_LINE_TOKEN"
}
```

Location of `configs.json` can be passed to the program through a command line argument.

LINE token can be obtained from https://notify-bot.line.me/ja/.