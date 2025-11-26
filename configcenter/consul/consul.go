package consul

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"

	consulApi "github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
	"github.com/zeromicro/go-zero/core/logx"
)

type (
	// ConsulSubscriber is a subscriber that subscribes to Consul.
	ConsulSubscriber struct {
		consulCli *consulApi.Client
		Path      string
		listeners []func()
		lock      sync.Mutex
		stopCh    chan struct{}
		Type      string
	}

	// ConsulConf is the configuration for Consul.
	ConsulConf Conf
)

// MustNewConsulSubscriber returns a Consul Subscriber, exits on errors.
func MustNewConsulSubscriber(conf ConsulConf) *ConsulSubscriber {
	s, err := NewConsulSubscriber(conf)
	logx.Must(err)
	return s
}

// NewConsulSubscriber returns a Consul Subscriber.
func NewConsulSubscriber(conf ConsulConf) (*ConsulSubscriber, error) {

	client, err := consulApi.NewClient(&consulApi.Config{
		Address:    conf.Host,
		Scheme:     conf.Scheme,
		PathPrefix: conf.PathPrefix,
		Datacenter: conf.Datacenter,
		Token:      conf.Token,
		TLSConfig:  conf.TLSConfig,
	})
	if err != nil {
		return nil, err
	}

	subscriber := &ConsulSubscriber{
		consulCli: client,
		Path:      conf.Key,
		stopCh:    make(chan struct{}),
		Type:      conf.Type,
	}
	go subscriber.watch()
	return subscriber, nil
}

// AddListener adds a listener to the subscriber.
func (s *ConsulSubscriber) AddListener(listener func()) error {

	s.lock.Lock()
	defer s.lock.Unlock()
	s.listeners = append(s.listeners, listener)
	return nil
}

// Value returns the current value from Consul.
func (s *ConsulSubscriber) Value() (string, error) {
	defaultConfig := viper.New()
	defaultConfig.SetConfigType(s.Type)
	kvPair, _, err := s.consulCli.KV().Get(s.Path, nil)
	if err != nil {
		return "", err
	}
	if kvPair == nil {
		return "", nil
	}
	err = defaultConfig.ReadConfig(bytes.NewBuffer(kvPair.Value))
	if err != nil {
		return "", err
	}
	settings := defaultConfig.AllSettings()
	marshal, _ := json.Marshal(settings)
	return string(marshal), nil
}

// watch monitors the Consul KV for changes and triggers listeners.
func (s *ConsulSubscriber) watch() {
	_, meta, err := s.consulCli.KV().Get(s.Path, nil)
	var lastIndex uint64
	if err == nil && meta != nil {
		lastIndex = meta.LastIndex
	}
	for {
		select {
		case <-s.stopCh:
			return
		default:
			_, meta, err := s.consulCli.KV().Get(s.Path, &consulApi.QueryOptions{
				WaitIndex: lastIndex,
			})
			if err != nil {
				time.Sleep(time.Second) // Retry after a delay
				continue
			}
			if meta.LastIndex > lastIndex {
				lastIndex = meta.LastIndex
				s.notifyListeners()
			}
		}
	}
}

// notifyListeners calls all registered listeners.
func (s *ConsulSubscriber) notifyListeners() {
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, listener := range s.listeners {
		listener()
	}
}

// Stop stops the watch process.
func (s *ConsulSubscriber) Stop() {
	close(s.stopCh)
}
