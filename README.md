# Cobweb
Distributed and equiped with proxypool spider system.


## Todo

- [ ] cobweb.Task 和 cobweb.Command 的信息追踪和获取
- [ ] Pipeline Item 的形式如何确定
    - 当前 Item 的类型为 `map[string]string`, 但这不能普遍适用. 如果某个 Field 需要使用字符串数组或者嵌套结构体就无法满足要求
- [ ] cobweb 的用例
    - [ ] douban top 250
        - 抓取信息有: 标题, 年份, 封面连接(并将封面保存), 导演, 编剧, 主演, 类型, 制片国家或地区, 语言, 片长, IMDb链接
    - [ ] 再加上两个用例
- [ ] 编写 ProxyPool 免费代理网站数据抓取规则




