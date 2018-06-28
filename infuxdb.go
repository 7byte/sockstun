package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/influxdata/influxdb/client/v2"
)

const (
	ssDB   = "ss_data"
	ssFlow = "flow"

	maxQueueLen = 10
)

// DataMeta 数据
type DataMeta struct {
	Host      string    // ss 服务器ip
	Port      int       // ss 服务器端口
	CAddr     string    // 客户端地址
	SAddr     string    // 服务端地址
	RLen      int64     // 请求长度
	WLen      int64     // 回复长度
	Timestamp time.Time // 时间戳
}

type influxdbClient struct {
	conf *influxDBConfig

	dataCh chan *DataMeta
	done   chan struct{}
}

var infuxdb *influxdbClient

func newInfluxdbClient(conf *influxDBConfig) (c *influxdbClient, err error) {
	if infuxdb != nil {
		return infuxdb, nil
	}
	if conf == nil {
		return
	}
	c = &influxdbClient{conf: conf}
	infuxdb = c

	c.dataCh = make(chan *DataMeta, maxQueueLen)
	c.done = make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go c.writeDataMetaRoutine(wg)
	wg.Wait()
	return
}

func (c *influxdbClient) newClient() (clnt client.Client, err error) {
	clnt, err = client.NewHTTPClient(client.HTTPConfig{
		Addr:     c.conf.Addr,
		Username: c.conf.User,
		Password: c.conf.Password,
	})
	if err != nil {
		logger.Errorf("New influxdb failed: %v", err)
		return
	}
	return
}

// queryDB convenience function to query the database
func queryDB(clnt client.Client, cmd string) (res []client.Result, err error) {
	q := client.Query{
		Command:  cmd,
		Database: ssDB,
	}
	if response, err := clnt.Query(q); err == nil {
		if response.Error() != nil {
			return res, response.Error()
		}
		res = response.Results
	} else {
		return res, err
	}
	return res, nil
}

func (c *influxdbClient) writeDataMeta(m *DataMeta) {
	select {
	case <-c.done:
		logger.Warning("This is impossible!")
		return
	default:
		c.dataCh <- m
	}
}

func (c *influxdbClient) onWrite(ms []*DataMeta) (err error) {
	clnt, err := c.newClient()
	if err != nil {
		return
	}
	defer clnt.Close()
	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database: ssDB,
	})
	for _, m := range ms {
		tags := map[string]string{
			"host": fmt.Sprint(m.Host),
			"port": fmt.Sprint(m.Port),
		}
		fields := map[string]interface{}{
			"caddr": m.CAddr,
			"saddr": m.SAddr,
			"rlen":  m.RLen,
			"wlen":  m.WLen,
		}
		pt, err := client.NewPoint(ssFlow, tags, fields, m.Timestamp)
		if err != nil {
			logger.Warning("New point failed: %v", err)
			continue
		}
		bp.AddPoint(pt)
	}
	if err = clnt.Write(bp); err != nil {
		logger.Warning("Write points err: %v", err)
		return
	}
	logger.Infof("onWrite %d records", len(ms))
	return
}

func (c *influxdbClient) writeDataMetaRoutine(wg *sync.WaitGroup) {
	wg.Done()
	bufferLen := maxQueueLen / 2
	ms := make([]*DataMeta, 0, bufferLen)
	for m := range c.dataCh {
		if len(ms) >= bufferLen {
			c.onWrite(ms)
			ms = ms[:0]
		}
		ms = append(ms, m)
	}
}
