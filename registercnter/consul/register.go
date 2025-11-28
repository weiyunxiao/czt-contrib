package consul

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/netx"
	"github.com/zeromicro/go-zero/core/proc"
)

type (
	Client interface {
		Register() error
		DeregisterService() error
		IsRegistered() (bool, error)
		GetServiceID() string
		GetRegistration() *api.AgentServiceRegistration
		UpdateStatus(status string) error
	}

	CommonClient struct {
		registration *api.AgentServiceRegistration
		apiClient    *api.Client
		serviceId    string
		serviceHost  string
		servicePort  int
		consulConf   Conf
		monitorFuncs []MonitorFunc
	}

	MonitorState struct {
		RetryCount     int
		BackoffTime    time.Duration
		MaxBackoffTime time.Duration
		MaxRetries     int
		Ticker         *time.Ticker
		OriginalTTL    time.Duration // 保存原始的TTL值，用于重置
		Mutex          sync.RWMutex
	}

	MonitorFunc func(cc *CommonClient, state *MonitorState) error

	ServiceOption func(*CommonClient)
)

func MustNewService(listenOn string, c Conf, opts ...ServiceOption) Client {
	service, err := NewService(listenOn, c, opts...)
	if err != nil {
		logx.Must(err)
	}
	return service
}

func NewService(listenOn string, c Conf, opts ...ServiceOption) (Client, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	pubListenOn := figureOutListenOn(listenOn)

	host, ports, err := net.SplitHostPort(pubListenOn)
	if err != nil {
		return nil, fmt.Errorf("failed parsing address error: %v", err)
	}
	port, _ := strconv.ParseUint(ports, 10, 16)

	client, err := api.NewClient(&api.Config{Scheme: c.Scheme, Address: c.Host, Token: c.Token})
	if err != nil {
		return nil, fmt.Errorf("create consul client error: %v", err)
	}

	service := &CommonClient{
		registration: nil,
		apiClient:    client,
		serviceId:    fmt.Sprintf("%s-%s-%d", c.Key, host, port),
		serviceHost:  host,
		servicePort:  int(port),
		consulConf:   c,
		monitorFuncs: make([]MonitorFunc, 0),
	}

	for _, option := range opts {
		option(service)
	}

	err = service.clientRegistration()
	if err != nil {
		return nil, err
	}

	return service, nil
}

func (cc *CommonClient) Register() error {
	err := cc.registerServiceWithHealthCheck()
	if err != nil {
		return err
	}

	switch cc.consulConf.CheckType {
	case checkTypeTTL:
		if len(cc.monitorFuncs) == 0 {
			cc.monitorFuncs = append(cc.monitorFuncs, ttlCheckMonitorFunc())
		}

		// 为每个监控函数启动独立的goroutine
		for _, monitorFunc := range cc.monitorFuncs {
			go cc.monitorServiceStatus(monitorFunc)
		}
	case checkTypeGrpc:
	case checkTypeHttp:
	default:
		return fmt.Errorf("unknown check type: %s", cc.consulConf.CheckType)
	}

	proc.AddShutdownListener(func() {
		err := cc.DeregisterService()
		if err != nil {
			logx.Errorf("deregister service %s error: %s", cc.serviceId, err.Error())
		} else {
			logx.Infof("Service %s deregistered successfully", cc.serviceId)
		}
	})
	return nil
}

func (cc *CommonClient) DeregisterService() error {
	err := cc.apiClient.Agent().ServiceDeregister(cc.serviceId)
	return err
}

func (cc *CommonClient) IsRegistered() (bool, error) {
	_, _, err := cc.apiClient.Agent().Service(cc.serviceId, nil)
	if err != nil {
		if strings.Contains(err.Error(), "Service not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to get service %s: %v", cc.serviceId, err)
	}
	return true, nil
}

func (cc *CommonClient) GetServiceID() string {
	return cc.serviceId
}

func (cc *CommonClient) UpdateStatus(status string) error {
	return cc.apiClient.Agent().UpdateTTL(cc.serviceId, "", status)
}

func (cc *CommonClient) GetRegistration() *api.AgentServiceRegistration {
	return cc.registration
}

func (cc *CommonClient) clientRegistration() error {
	reg := &api.AgentServiceRegistration{
		ID:      cc.serviceId,       // Service node name
		Name:    cc.consulConf.Key,  // Service name
		Tags:    cc.consulConf.Tag,  // Tags, can be empty
		Meta:    cc.consulConf.Meta, // Meta, can be empty
		Port:    cc.servicePort,     // Service port
		Address: cc.serviceHost,     // Service IP
	}

	switch cc.consulConf.CheckType {
	case checkTypeTTL:
		reg.Checks = []*api.AgentServiceCheck{
			{
				CheckID:                        cc.serviceId,                          // Service node name
				TTL:                            fmt.Sprintf("%ds", cc.consulConf.TTL), // Health check interval
				Status:                         "passing",
				DeregisterCriticalServiceAfter: fmt.Sprintf("%ds", cc.consulConf.TTL*cc.consulConf.ExpiredTTL), // Deregistration time
			},
		}

	case checkTypeGrpc:
	case checkTypeHttp:
		// todo 可以考虑混合健康检查，例如TTL和HTTP
		httpCheckHost := figureOutListenOn(cc.consulConf.CheckHttp.Host)
		reg.Checks = []*api.AgentServiceCheck{
			{
				CheckID:                        cc.serviceId,                                                     // Service node name
				HTTP:                           fmt.Sprintf("%s%s", httpCheckHost, cc.consulConf.CheckHttp.Path), // health check url
				Method:                         cc.consulConf.CheckHttp.Method,                                   // health check method
				Interval:                       fmt.Sprintf("%ds", cc.consulConf.TTL),                            // health check interval
				Timeout:                        fmt.Sprintf("%ds", cc.consulConf.CheckTimeout),                   // health check timeout
				DeregisterCriticalServiceAfter: fmt.Sprintf("%ds", cc.consulConf.TTL*cc.consulConf.ExpiredTTL),   // Deregistration time
			},
		}
	default:
		return fmt.Errorf("unknown check type: %s", cc.consulConf.CheckType)
	}
	cc.registration = reg
	return nil
}

func (cc *CommonClient) registerServiceWithHealthCheck() error {

	err := cc.DeregisterService()

	if err != nil {
		logx.Errorf("deregister service %s error: %s", cc.serviceId, err.Error())
	} else {
		logx.Infof("Service %s deregistered successfully", cc.serviceId)
	}

	time.Sleep(100 * time.Millisecond)

	err = cc.registerService()
	if err != nil {
		return err
	}

	// health check register
	err = cc.serviceHealthCheck()
	if err != nil {
		_ = cc.DeregisterService()
		return fmt.Errorf("register service %s id %s check error: %v",
			cc.registration.Name, cc.registration.ID, err)
	}

	logx.Infof("Service %s id %s registered successfully", cc.registration.Name, cc.registration.ID)

	return nil
}

func (cc *CommonClient) registerService() error {
	err := cc.apiClient.Agent().ServiceRegister(cc.registration)
	if err != nil {
		return fmt.Errorf("initial register service %s id %s host to consul error: %s",
			cc.registration.Name, cc.registration.ID, err.Error())
	}
	return nil
}

func (cc *CommonClient) serviceHealthCheck() error {
	check := api.AgentServiceCheck{TTL: fmt.Sprintf("%ds", cc.consulConf.TTL),
		Status: "passing", DeregisterCriticalServiceAfter: fmt.Sprintf("%ds", cc.consulConf.TTL*cc.consulConf.ExpiredTTL)}
	err := cc.apiClient.Agent().CheckRegister(&api.AgentCheckRegistration{
		ID:                cc.registration.ID,
		Name:              cc.registration.Name,
		ServiceID:         cc.registration.ID,
		AgentServiceCheck: check,
	})
	return err
}

func (cc *CommonClient) monitorServiceStatus(monitorFunc MonitorFunc) {

	stopCh := make(chan struct{})

	proc.AddShutdownListener(func() {
		logx.Infof("Stopping service monitor for %s", cc.serviceId)
		close(stopCh)
	})

	var ttlTicker time.Duration
	if cc.consulConf.TTL > 0 {
		ttlTicker = time.Duration(cc.consulConf.TTL-1) * time.Second
		if ttlTicker < time.Second {
			ttlTicker = time.Second
		}
	} else {
		// 默认值
		ttlTicker = 1 * time.Second
	}

	state := &MonitorState{
		RetryCount:     0,
		BackoffTime:    1 * time.Second,
		MaxRetries:     5,
		MaxBackoffTime: 30 * time.Second,
		OriginalTTL:    ttlTicker,
		Ticker:         time.NewTicker(ttlTicker),
	}
	defer state.Ticker.Stop()

	for {
		select {
		case <-state.Ticker.C:
			state.Mutex.Lock()
			err := monitorFunc(cc, state)
			state.Mutex.Unlock()

			if err != nil {
				logx.Errorf("Monitor function error for service %s: %v", cc.serviceId, err)
			}
		case <-stopCh:
			logx.Infof("Service monitor for %s stopped gracefully", cc.serviceId)
			return
		}
	}

}

func ttlCheckMonitorFunc() MonitorFunc {
	return func(cc *CommonClient, state *MonitorState) error {

		err := cc.apiClient.Agent().UpdateTTL(cc.serviceId, "", "passing")
		if err == nil {
			logx.Infof("Service %s TTL updated successfully", cc.serviceId)
			state.RetryCount = 0
			state.BackoffTime = 1 * time.Second
			state.Ticker.Reset(state.OriginalTTL)
			return nil
		}

		registered, err := cc.checkServiceRegistration()
		if err != nil {
			logx.Error(err)
		}

		if !registered && state.RetryCount < state.MaxRetries {
			// 尝试重新注册服务
			logx.Infof("Attempting to re-register service %s (retry %d/%d)...", cc.serviceId, state.RetryCount+1, state.MaxRetries)

			err = cc.registerServiceWithHealthCheck()
			if err != nil {
				logx.Errorf("Failed to re-register service %s: %v. Retrying in %v", cc.serviceId, err, state.BackoffTime)
				state.RetryCount++

				// 调整定时器实现退避
				state.Ticker.Reset(state.BackoffTime)
				nextBackoffTime := state.BackoffTime * 2
				if nextBackoffTime > state.MaxBackoffTime {
					nextBackoffTime = state.MaxBackoffTime
				}
				state.BackoffTime = nextBackoffTime
				return err
			}

			logx.Infof("Service %s re-registered successfully", cc.serviceId)
			state.RetryCount = 0
			state.BackoffTime = 1 * time.Second
			state.Ticker.Reset(state.OriginalTTL) // 使用保存的原始TTL值
			return nil
		}

		if !registered {
			logx.Errorf("Max retries reached for service %s. Resetting retry counter and backoff time.", cc.serviceId)
			state.RetryCount = 0
			state.BackoffTime = 1 * time.Second
			state.Ticker.Reset(state.BackoffTime)
		}

		return nil
	}
}

func (cc *CommonClient) checkServiceRegistration() (bool, error) {

	service, _, err := cc.apiClient.Agent().Service(cc.serviceId, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get service %s: %v", cc.serviceId, err)
	}

	serviceEntries, _, err := cc.apiClient.Health().Service(service.Service, "", false, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get service %s health: %v", cc.serviceId, err)
	}

	for _, entry := range serviceEntries {
		if entry.Service.ID == cc.serviceId {
			status := entry.Checks.AggregatedStatus()
			return status == api.HealthPassing, nil
		}
	}
	return false, fmt.Errorf("service with ID %s not found in health check results", cc.serviceId)

}

func figureOutListenOn(listenOn string) string {
	fields := strings.Split(listenOn, ":")
	if len(fields) == 0 {
		return listenOn
	}

	host := fields[0]
	if len(host) > 0 && host != allEths {
		return listenOn
	}

	ip := os.Getenv(envPodIP)
	if len(ip) == 0 {
		ip = netx.InternalIp()
	}
	if len(ip) == 0 {
		return listenOn
	}

	return strings.Join(append([]string{ip}, fields[1:]...), ":")
}

func WithMonitorFuncs(funcs ...MonitorFunc) ServiceOption {
	return func(cc *CommonClient) {
		cc.monitorFuncs = append(cc.monitorFuncs, funcs...)
	}
}
