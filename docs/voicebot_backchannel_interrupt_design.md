# 工业级 Voicebot Backchannel + Interrupt 设计

## 1. 总体原则
- 用户短词输入可能是：Backchannel ACK / INTERRUPT / NEW_QUESTION / IGNORE_NOISE
- 打断分三种 span：LLM chat / TTS synthesize / TTS play
- 上下文管理基于 commit timing 与播放进度
- 噪声 / filler 输入不会影响正在播放或生成的 span

---

## 2. 各 Span 打断处理

### 2.1 LLM chat span / TTS synthesize span
- 用户输入 → 判定有效性（interrupt / filler / noise / ack）
- IGNORE_NOISE / FILLER → 忽略，不 cancel span
- INTERRUPT / NEW_QUESTION → cancel span，不 commit 当前 response
- 连续用户输入 → 可触发 repair agent
- 下一轮用户输入触发新 LLM chat

### 2.2 TTS play span
- 用户输入 → Backchannel 判定
  - 使用短词 + 最近已 commit 上下文 + VAD gap + 噪声/发音特征
  - 分类结果：BACKCHANNEL_ACK / INTERRUPT / NEW_QUESTION / IGNORE_NOISE
- 播放时长判断：

| 播放时长 | 判定结果 | 动作 | 上下文更新 | 下一步 |
|-----------|-----------|------|------------|--------|
| <2s | IGNORE_NOISE / FILLER | 继续播放，不影响当前 TTS | 无 | 等待继续播放 / 后续输入判定 |
| <2s | INTERRUPT / NEW_QUESTION | 停止播放，丢弃当前 LLM response | 不提交 | 用户最新输入触发新一轮 LLM chat |
| ≥2s | IGNORE_NOISE / FILLER | 继续播放，不影响当前 TTS | 无 | 等待播放完成 |
| ≥2s | INTERRUPT / NEW_QUESTION | 停止播放 | 已播放句子分段 commit 到上下文 | 用户最新输入触发新一轮 LLM chat |

- 核心原则：
  - <2s 播放时，Backchannel 判定上下文只使用已 commit 内容
  - ≥2s 播放时，已播放句子可用于上下文 commit
  - IGNORE_NOISE / FILLER 不影响播放

---

## 3. Repair Agent
- 连续多轮未 commit 用户输入触发
- 整合缺失语义片段，生成完整 prompt 送主 LLM
- 可选触发，不必每次都启动

---

## 4. 噪声 / Filler 处理
- RMS / SNR / duration + 小 LLM 判定
- 极短 filler (<0.1~0.2s) → 自动 ignore
- 连续 filler → 可触发 repair agent整合

---

## 5. 接口设计建议
- 可以设计统一接口，内部策略根据 spanType 区分：

```go
BackchannelChecker.check(userUtterance, spanType, context, vadGap, audioFeatures) -> Decision
```
- spanType = LLM_CHAT | TTS_SYNTH | TTS_PLAY
- 内部策略：
  - LLM_CHAT / TTS_SYNTH → 轻量 interrupt 判定
  - TTS_PLAY → commit timing + 已播放句子判定 + interrupt 判定

---

## 6. 总结
- 核心原则是“用户输入打断 span 时，先判定类型（backchannel / noise / new question），再根据播放进度决定 commit 或丢弃，再触发新一轮 LLM chat”，同时保持历史上下文完整和噪声安全过滤。

