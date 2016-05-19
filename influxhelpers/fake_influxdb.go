package influxhelpers

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
)

type FakeInfluxDB struct {
	server           *httptest.Server
	ReceivedContents chan []byte
}

func NewFakeInfluxDB() *FakeInfluxDB {
	return &FakeInfluxDB{
		ReceivedContents: make(chan []byte, 100),
	}
}

func (f *FakeInfluxDB) Start() {
	f.server = httptest.NewUnstartedServer(f)
	f.server.Start()
}

func (f *FakeInfluxDB) Close() {
	f.server.Close()
}

func (f *FakeInfluxDB) URL() string {
	return f.server.URL
}

func (f *FakeInfluxDB) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	contents, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	go func() {
		f.ReceivedContents <- contents
	}()
}
