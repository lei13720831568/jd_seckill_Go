package cmd

import (
	"errors"
	"fmt"
	"github.com/Albert-Zhan/httpc"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
	"github.com/ztino/jd_seckill/common"
	"github.com/ztino/jd_seckill/jd_seckill"
	"log"
	"net/http"
	"os"
	"time"
)

func init() {
	rootCmd.AddCommand(seckillCmd)
}

var seckillCmd = &cobra.Command{
	Use:   "seckill",
	Short: "Start panic buying procedure",
	Run: startSeckill,
}

func startSeckill(cmd *cobra.Command, args []string)  {
	session:=jd_seckill.NewSession(common.CookieJar)
	err:=session.CheckLoginStatus()
	if err!=nil {
		log.Println("抢购失败，请重新登录")
	}else{
		//活跃用户会话,当会话失效自动退出程序
		user:=jd_seckill.NewUser(common.Client,common.Config)
		go KeepSession(user)
		//计算抢购时间
		nowLocalTime:=time.Now().UnixNano()/1e6
		jdTime,_:=GetJdTime()
		buyDate:=common.Config.MustValue("config","buy_time","")
		loc, _ := time.LoadLocation("Local")
		t,_:=time.ParseInLocation("2006-01-02 15:04:05",buyDate,loc)
		buyTime:=t.UnixNano()/1e6
		diffTime:=nowLocalTime-jdTime
		log.Println(fmt.Sprintf("正在等待到达设定时间:%s，检测本地时间与京东服务器时间误差为【%d】毫秒",buyDate,diffTime))
		timerTime:=(buyTime+diffTime)-jdTime
		if timerTime<=0 {
			log.Println("请设置抢购时间")
			os.Exit(0)
		}
		//等待抢购
		time.Sleep(time.Duration(timerTime)*time.Millisecond)
		//开始抢购
		log.Println("时间到达，开始执行……")
		seckill:=jd_seckill.NewSeckill(common.Client,common.Config)
		//开启抢购任务,第二个参数为开启几个协程
		//怕封号的可以减少协程数量,相反抢到的成功率也减低了
		Start(seckill,5)
	}
}

func GetJdTime() (int64,error) {
	req:=httpc.NewRequest(common.Client)
	resp,body,err:=req.SetUrl("https://a.jd.com//ajax/queryServerData.html").SetMethod("get").Send().End()
	if err!=nil || resp.StatusCode!=http.StatusOK {
		log.Println("获取京东服务器时间失败")
		return 0,errors.New("获取京东服务器时间失败")
	}
	return gjson.Get(body,"serverTime").Int(),nil
}

func Start(seckill *jd_seckill.Seckill,taskNum int)  {
	seckillTotalTime:=time.Now().Add(2*time.Minute).Unix()
	//开始检测抢购状态
	go CheckSeckillStatus()
	//抢购总时间两分钟,超时程序自动退出
	for time.Now().Unix()<seckillTotalTime {
		for i:=1;i<=taskNum;i++ {
			go task(seckill)
		}
		//每隔1.5秒执行一次
		//怕封号的可以增加间隔时间,相反抢到的成功率也减低了
		time.Sleep(1500*time.Millisecond)
	}
	log.Println("抢购结束，具体详情请查看日志")
}

func task(seckill *jd_seckill.Seckill)  {
	seckill.RequestSeckillUrl()
	seckill.SeckillPage()
	flag:=seckill.SubmitSeckillOrder()
	//提前抢购成功的,直接结束程序
	if flag {
		//通知管道
		common.SeckillStatus<-true
	}
}

func CheckSeckillStatus()  {
	for {
		select {
		case <-common.SeckillStatus:
			//抢购成功,程序退出
			os.Exit(0)
		}
	}
}

func KeepSession(user *jd_seckill.User)  {
	//每30分钟检测一次
	t:=time.NewTicker(30*time.Minute)
	for {
		select {
		case <-t.C:
			if err:=user.RefreshStatus();err!=nil {
				_=os.Remove("./cookie.txt")
				log.Println("会话失效,程序自动退出")
				os.Exit(0)
			}
			log.Println("活跃会话成功")
		}
	}
}