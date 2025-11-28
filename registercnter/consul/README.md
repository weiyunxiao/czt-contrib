## 默认

```api
client := consul.MustNewService(":8080", consulConfig)
```

## 单个

```api
customMonitor := func(cc *consul.CommonClient, state *consul.MonitorState) error {
    // 自定义监控逻辑
    return nil
}

client := consul.MustNewService(":8080", consulConfig, 
    consul.WithMonitorFuncs(customMonitor))
```

## 多个

```api
monitor1 := func(cc *consul.CommonClient, state *consul.MonitorState) error {
    // 监控逻辑1
    return nil
}

monitor2 := func(cc *consul.CommonClient, state *consul.MonitorState) error {
    // 监控逻辑2
    return nil
}

client := consul.MustNewService(":8080", consulConfig, 
    consul.WithMonitorFuncs(monitor1, monitor2))
```