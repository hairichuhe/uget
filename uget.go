package uget

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/asaskevich/govalidator" //验证器，验证strings等类型
	"github.com/pkg/errors"
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

	if err := uget.Checking(); err != nil {
		return errors.Wrap(err, "failed to check header")
	}

	if err := uget.Download(); err != nil {
		return err
	}

	//	if err := uget.Utils.BindwithFiles(uget.Procs); err != nil {
	//		return err
	//	}

	return nil
}

// Ready method define the variables required to Download.定义下载所需要的变量
func (uget *Uget) Ready() error {
	//设置可执行的最大cpu数
	if procs := os.Getenv("GOMAXPROCS"); procs == "" {
		runtime.GOMAXPROCS(uget.Procs)
	}

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
