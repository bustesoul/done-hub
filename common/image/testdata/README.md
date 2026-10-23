# 固定图片样本

`sample.jpeg` (9×6)、`sample.png` (7×5)、`sample.gif` (5×4) 和 `small.jpeg` (270×230)
由 Go 标准库编码器生成。像素 RGB 为 `(uint8(x*23), uint8(y*37), 99)`，透明度 255；编码器使用默认选项。

`sample.webp` (1080×720) 复制自本项目的 `web/src/assets/images/invite/cover.webp`。
固定副本使测试不依赖前端资源路径、后续界面改动或公网下载。
