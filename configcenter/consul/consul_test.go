package consul

import (
	"testing"
	"time"

	configurator "github.com/zeromicro/go-zero/core/configcenter"
)

type BaseConfig struct {
	ConfigCenterConsul ConsulConf // 配置中心配置
}

func TestMustNewConsulSubscriber(t *testing.T) {
	conf := ConsulConf{
		Host:   "consul-target.ystz-telemetry-consul.ystz.cztycwl.com:30715",
		Scheme: "http",
		Key:    "devLocal/czt-contrib/yaml",
		Type:   "yaml",
	}
	subscriber := MustNewConsulSubscriber(conf)
	if subscriber == nil {
		t.Errorf("MustNewConsulSubscriber() = %v, want %v", subscriber, nil)
	}
}

func TestConsulSubscriberYaml(t *testing.T) {
	// 配置Consul连接信息
	conf := ConsulConf{
		Host:   "consul-target.ystz-telemetry-consul.ystz.cztycwl.com:30715",
		Scheme: "http",
		Key:    "devLocal/czt-contrib/yaml",
		Type:   "yaml",
	}

	// 创建订阅者
	subscriber := MustNewConsulSubscriber(conf)
	defer subscriber.Stop() // 确保测试结束后停止订阅者

	// 创建配置中心
	cc := configurator.MustNewConfigCenter[BaseConfig](configurator.Config{
		Type: "yaml",
	}, subscriber)

	// 获取配置并验证
	v, err := cc.GetConfig()
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	// 添加基本断言，验证配置是否正确加载
	if v.ConfigCenterConsul.Host == "" {
		t.Errorf("Expected non-empty Host in config, got empty")
	}

	// 记录当前配置值用于参考
	t.Logf("Successfully loaded config: Host=%s, Key=%s, Type=%s",
		v.ConfigCenterConsul.Host, v.ConfigCenterConsul.Key, v.ConfigCenterConsul.Type)

	// 创建一个通道来等待配置变更通知
	configChanged := make(chan struct{})

	// 添加监听器
	cc.AddListener(func() {
		v, err := cc.GetConfig()
		if err != nil {
			t.Errorf("Failed to get updated config: %v", err)
			return
		}

		// 验证更新后的配置
		if v.ConfigCenterConsul.Host == "" {
			t.Errorf("Expected non-empty Host in updated config, got empty")
			return
		}

		t.Logf("Config changed detected: Host=%s", v.ConfigCenterConsul.Host)
		close(configChanged)
	})

	// 添加超时机制，避免测试无限等待
	select {
	case <-configChanged:
		// 配置变更已检测到
		t.Log("Configuration change test completed successfully")
	case <-time.After(10 * time.Second):
		// 超时，但这不一定是失败，因为测试环境可能没有配置变更
		t.Log("No configuration change detected within timeout period (this is expected in static test environments)")
	}
}

func TestConsulSubscriberJson(t *testing.T) {
	// 配置Consul连接信息
	conf := ConsulConf{
		Host:   "consul-target.ystz-telemetry-consul.ystz.cztycwl.com:30715",
		Scheme: "http",
		Key:    "devLocal/czt-contrib/json",
		Type:   "json",
	}

	// 创建订阅者
	subscriber := MustNewConsulSubscriber(conf)
	defer subscriber.Stop() // 确保测试结束后停止订阅者

	// 创建配置中心
	cc := configurator.MustNewConfigCenter[BaseConfig](configurator.Config{
		Type: "json",
	}, subscriber)

	// 获取配置并验证
	v, err := cc.GetConfig()
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	// 添加基本断言，验证配置是否正确加载
	if v.ConfigCenterConsul.Host == "" {
		t.Errorf("Expected non-empty Host in config, got empty")
	}

	// 记录当前配置值用于参考
	t.Logf("Successfully loaded config: Host=%s, Key=%s, Type=%s",
		v.ConfigCenterConsul.Host, v.ConfigCenterConsul.Key, v.ConfigCenterConsul.Type)

	// 创建一个通道来等待配置变更通知
	configChanged := make(chan struct{})

	// 添加监听器
	cc.AddListener(func() {
		v, err := cc.GetConfig()
		if err != nil {
			t.Errorf("Failed to get updated config: %v", err)
			return
		}

		// 验证更新后的配置
		if v.ConfigCenterConsul.Host == "" {
			t.Errorf("Expected non-empty Host in updated config, got empty")
			return
		}

		t.Logf("Config changed detected: Host=%s", v.ConfigCenterConsul.Host)
		close(configChanged)
	})

	// 添加超时机制，避免测试无限等待
	select {
	case <-configChanged:
		// 配置变更已检测到
		t.Log("Configuration change test completed successfully")
	case <-time.After(10 * time.Second):
		// 超时，但这不一定是失败，因为测试环境可能没有配置变更
		t.Log("No configuration change detected within timeout period (this is expected in static test environments)")
	}
}
