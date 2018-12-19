package uget

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/andlabs/ui"
	"github.com/asaskevich/govalidator" //验证器，验证strings等类型
	"github.com/pkg/errors"
	"github.com/tealeg/xlsx"
)

const (
	version = "0.0.6"
	msg     = "Uget v" + version + ", parallel file download client\n"
)

// Uget structs
type Uget struct {
	Utils             //文件相关数据，是 一个接口，只要相应对象实现它所拥有的方法即可
	TargetDir  string //目标路径
	Procs      int    //启用cpu数目
	URLs       []string
	TargetURLs []string
	args       []string
	timeout    int
	useragent  string
	referer    string
}

type ignore struct {
	err error
}

type cause interface {
	Cause() error
}

var excelSrc = make([]string, 0)
var cnum = make(chan int, 1) //一个协程在跑
var pbc = make(chan int)

// New for uget package
func New() *Uget {
	return &Uget{
		Utils:   &Data{},
		Procs:   runtime.NumCPU(), // default 8核16线程，指的是线程数
		timeout: 10,
	}
}

// ErrTop get important message from wrapped error message 从包裹的重要信息中获取重要消息
func (uget Uget) ErrTop(err error) error {
	for e := err; e != nil; {
		switch e.(type) {
		case ignore:
			return nil
		case cause:
			e = e.(cause).Cause()
		default:
			return e
		}
	}

	return nil
}

// Run execute methods in uget package
func (uget *Uget) Run() error {
	//	if err := uget.Ready(); err != nil {
	//		return uget.ErrTop(err)
	//	}

	//	if err := uget.Utils.BindwithFiles(uget.Procs); err != nil {
	//		return err
	//	}
	if err := uget.StartGui(); err != nil {
		return uget.ErrTop(err)
	}

	return nil
}

//界面初始化
func (uget *Uget) StartGui() error {
	//设置可执行的最大cpu数
	if procs := os.Getenv("GOMAXPROCS"); procs == "" {
		runtime.GOMAXPROCS(uget.Procs)
	}
	err := ui.Main(func() {
		button := ui.NewButton("请选择您的数据文件（.xlsx）")
		filelabel := ui.NewLabel("")
		box := ui.NewVerticalBox()
		box.SetPadded(true)
		box.Append(button, false)
		box.Append(filelabel, false)

		hbox := ui.NewHorizontalBox()
		hbox.SetPadded(true)
		templatebutton := ui.NewButton("下载模板")
		generatebutton := ui.NewButton("开始下载")
		hbox.Append(templatebutton, false)
		hbox.Append(generatebutton, false)

		box.Append(hbox, false)

		resultlabel := ui.NewLabel("")
		pbar := ui.NewProgressBar()
		box.Append(resultlabel, false)
		box.Append(pbar, false)
		pbar.Hide()

		window := ui.NewWindow("批量下载器", 600, 200, false)
		filewindow := ui.NewWindow("请选择生成数据文件", 600, 200, false)
		window.SetMargined(true)
		window.SetChild(box)
		button.OnClicked(func(*ui.Button) {
			resultlabel.SetText("")
			s := ui.OpenFile(filewindow)
			if s != "" {
				if strings.HasSuffix(s, ".xlsx") {
					filelabel.SetText("您选择的数据文件为：" + s)
				} else {
					filelabel.SetText("请选择类型为xlsx的文件！")
				}
			}
		})
		templatebutton.OnClicked(func(*ui.Button) {
			cmd := exec.Command("cmd", "/C", "start http://111.11.157.20/file/template/%E6%89%B9%E9%87%8F%E4%B8%8B%E8%BD%BD%E6%A8%A1%E6%9D%BF.xlsx")
			cmd.Run()
		})
		generatebutton.OnClicked(func(*ui.Button) {
			fls := filelabel.Text()
			if fls == "" || fls == "请选择类型为xlsx的文件！" {
				ui.MsgBox(window, "请选择正确的xlsx文件！", "如果不知怎么使用，请下载模板导入，如果还是不会，请加qq：14320794寻求技术指导！")
			} else {
				resultlabel.SetText("文件下载中…")
				generatebutton.Disable()
				pbar.Show()
				flsrc := strings.Replace(fls, "您选择的数据文件为：", "", -1)
				err1 := readExcel(flsrc)
				if err1 == nil {
					go uget.createxc()
					checkProgress(func(current, total int) {
						ui.QueueMain(func() {
							value := int(float64(current) / float64(total) * 100.0)
							pbar.SetValue(value)
							if current == total {
								resultlabel.SetText("文件下载成功!")
								generatebutton.Enable()
								pbar.Hide()
							}
						})
					})
				} else {
					resultlabel.SetText("生成出错：" + err1.Error())
					generatebutton.Enable()
					pbar.Hide()
				}
			}
		})
		window.OnClosing(func(*ui.Window) bool {
			ui.Quit()
			return true
		})
		window.Show()
	})
	return err
}

// Ready method define the variables required to Download.定义下载所需要的变量
func (uget *Uget) Ready() error {

	var opts Options //命令行参数
	if err := uget.parseOptions(&opts, os.Args[1:]); err != nil {
		return errors.Wrap(err, "failed to parse command line args")
	}

	if opts.Procs > 2 {
		uget.Procs = opts.Procs
	}

	if opts.Timeout > 0 {
		uget.timeout = opts.Timeout
	}

	//解析下载内容
	if err := uget.parseURLs(); err != nil {
		return errors.Wrap(err, "failed to parse of url")
	}

	if opts.Output != "" {
		uget.Utils.SetFileName(opts.Output)
	}

	if opts.UserAgent != "" {
		uget.useragent = opts.UserAgent
	}

	if opts.Referer != "" {
		uget.referer = opts.Referer
	}

	if opts.TargetDir != "" {
		info, err := os.Stat(opts.TargetDir)
		if err != nil {
			if !os.IsNotExist(err) {
				return errors.Wrap(err, "target dir is invalid")
			}

			if err := os.MkdirAll(opts.TargetDir, 0755); err != nil {
				return errors.Wrapf(err, "failed to create diretory at %s", opts.TargetDir)
			}

		} else if !info.IsDir() {
			return errors.New("target dir is not a valid directory")
		}
		opts.TargetDir = strings.TrimSuffix(opts.TargetDir, "/")
	}
	uget.TargetDir = opts.TargetDir

	return nil
}

func (uget Uget) makeIgnoreErr() ignore {
	return ignore{
		err: errors.New("this is ignore message"),
	}
}

// Error for options: version, usage
func (i ignore) Error() string {
	return i.err.Error()
}

func (i ignore) Cause() error {
	return i.err
}

func (uget *Uget) parseOptions(opts *Options, argv []string) error {

	if len(argv) == 0 {
		os.Stdout.Write(opts.usage()) //写入使用信息
		return uget.makeIgnoreErr()
	}

	//进行参数解析
	o, err := opts.parse(argv)
	if err != nil {
		return errors.Wrap(err, "failed to parse command line options")
	}

	if opts.Help {
		os.Stdout.Write(opts.usage())
		return uget.makeIgnoreErr()
	}

	if opts.Version {
		os.Stdout.Write([]byte(msg))
		return uget.makeIgnoreErr()
	}

	if opts.Update {
		result, err := opts.isupdate()
		if err != nil {
			return errors.Wrap(err, "failed to parse command line options")
		}

		os.Stdout.Write(result)
		return uget.makeIgnoreErr()
	}

	uget.args = o

	return nil
}

func (uget *Uget) parseURLs() error {

	// find url in args
	for _, argv := range uget.args {
		if govalidator.IsURL(argv) {
			uget.URLs = append(uget.URLs, argv)
		}
	}

	if len(uget.URLs) < 1 {
		fmt.Fprintf(os.Stdout, "Please input url separate with space or newline\n")
		fmt.Fprintf(os.Stdout, "Start download at ^D\n")

		// scanning url from stdin
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			scan := scanner.Text()
			urls := strings.Split(scan, " ")
			for _, url := range urls {
				if govalidator.IsURL(url) {
					uget.URLs = append(uget.URLs, url)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return errors.Wrap(err, "failed to parse url from stdin")
		}

		if len(uget.URLs) < 1 {
			return errors.New("urls not found in the arguments passed")
		}
	}

	return nil
}

func readExcel(src string) error {
	xlFile, err := xlsx.OpenFile(src)
	if err != nil {
		return err
	}
	for _, sheet := range xlFile.Sheets {
		for i, row := range sheet.Rows {
			if i != 0 && row.Cells[0].String() != "" {
				excelSrc = append(excelSrc, row.Cells[0].String())
			}
		}
	}
	return nil
}

func (uget *Uget) createxc() {
	for i := 0; i < len(excelSrc); i++ {
		cnum <- 1
		go uget.download(excelSrc[i])
	}
}

func (uget *Uget) download(o string) {

	//	if err := uget.Checking(); err != nil {
	//		return errors.Wrap(err, "failed to check header")
	//	}

	//	if err := uget.Download(); err != nil {
	//		return err
	//	}
	//	if err := uget.Utils.BindwithFiles(uget.Procs); err != nil {
	//		return err
	//	}
	uget.Checking()
	uget.Download()
	uget.Utils.BindwithFiles(uget.Procs)

	<-cnum
	pbc <- 1
}

type progress func(current, total int)

func checkProgress(p progress) {
	go func() {
		for i, _ := range excelSrc {
			<-pbc
			p(i+1, len(excelSrc))
		}
	}()
}
