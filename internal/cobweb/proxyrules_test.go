package cobweb

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestProxyRules(t *testing.T) {
	dbStorageOnce.Do(func() {
		absStorageSingleton = &dbStorageForProxyRuleTest{}
	})
	data := []struct {
		rule     BaseRule
		filename string
	}{
		{&kuaidailiRule{}, "kuaidaili.html"},
		{&yundailiRule{}, "yundaili.html"},
		{&mianfeidaili89Rule{}, "89ip.html"},
		//{&quanwangdailiRule{}, "quanwangdaili.html"},
		{&kaixindailiRule{}, "kaixindaili.html"},
		{&xiaohuandailiRule{}, "xiaohuandaili.html"},
	}
	suit := NewTestSuits(t)
	for _, datum := range data {
		fmt.Printf("-------------- %v ----------------\n", datum.filename)
		suit.WithFile(datum.rule.InitParse, filepath.Join("proxypages", datum.filename), H{})
	}
}

type dbStorageForProxyRuleTest struct {
}

func (s *dbStorageForProxyRuleTest) GetProxy(proxy *Proxy) (*Proxy, error) {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) HasProxy(proxy *Proxy) (bool, error) {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) CreateProxy(proxy *Proxy) error {
	fmt.Println(proxy.Json())
	return nil
}

func (s *dbStorageForProxyRuleTest) CreateProxyList(proxies []*Proxy) int {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) ActivateProxy(proxy *Proxy) error {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) DeactivateProxy(proxy *Proxy) error {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) GetTopKProxyList(k int) ([]*Proxy, error) {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) GetRandTopKProxy(k int) (*Proxy, error) {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) GetAllProxy() ([]*Proxy, error) {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) GetProxyListWithRefuseList(refuseList []*Proxy, count int) ([]*Proxy, error) {
	panic("implement me")
}

func (s *dbStorageForProxyRuleTest) GetRandProxyWithRefuseList(proxies []*Proxy) (*Proxy, error) {
	panic("implement me")
}
