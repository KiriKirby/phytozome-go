# BLAST 外部参考器

BLAST 的外部参考器是可选的证据层。三个顶层开关（自动识别 BLAST hit symbol name、UniProt、InterPro）现在默认全部关闭；用户可以分别开启，不会因为关闭其中一个而影响其他参考器。

## InterPro 的当前范围

启用 InterPro 后，PHgo 会对 BLAST 原序列和每个 BLAST hit 使用官方 EBI InterPro API，并保留七个活动列：

- InterPro entry name
- InterPro entry type
- InterPro coverage (%)
- InterPro match regions
- InterPro accessions
- InterPro signature accessions
- InterPro Pfam accessions

旧的 `InterPro conserved region status` 以及它的阈值和判定代码仍保留在源码与快照模型中，用于兼容历史数据，但当前流程不会执行、不会生成、不会显示这一列。

## 原序列对比开关

InterPro 设置中的“Compare the seven InterPro columns with the original query”默认开启。关闭后仍会正常查询和显示七列，只是不显示颜色区别。

- entry name、entry type、accessions、signature accessions、Pfam accessions：与原序列完全相同为绿色；包含原序列全部项目但多出项目为黄色。
- coverage：命中值为 100% 为绿色。
- match regions：项目和长度都相同为绿色；存在相同项目且长度相同但还有差异为黄色；项目相同但对应长度不同为红色。

这些颜色是人工复核提示，不代表功能、正交关系或生物学活性的证明。

## 原序列元数据

BLAST Excel 导出新增/扩展 `Query Metadata` sheet，包含原序列的标识符、`phgo_alias`、来源、长度、原始序列，以及已经获取到的完整 UniProt/InterPro 值。详情弹窗的第二个 tab 是 `Query Metadata`，随后才是 UniProt、InterPro 和 FASTA；原序列的 motif 匹配和外部参考值也放在这里。

InterPro 查询使用内存/磁盘缓存和去重并发查找；缓存只影响速度，不改变结果含义。
