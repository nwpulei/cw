flowchart TD
Start([Feed 调用]) --> InputCheck{State == OFF?}

    %% --- 分支 1: 空窗累加 ---
    InputCheck -- Yes (空窗) --> AccumulateGap[d.lastGapDuration += duration]
    AccumulateGap --> RetEmpty([Return ""])

    %% --- 分支 2: 信号处理 ---
    InputCheck -- No (信号 On) --> StitchCheck{"d.lastGap < GlitchThreshold?<br>(且 > 0)"}

    %% --- 噪声缝合逻辑 ---
    StitchCheck -- Yes (是毛刺空窗) --> Stitching[<b>缝合操作</b><br>pendingMark += lastGap + currentMark<br>lastGap = 0]
    Stitching --> RetEmpty

    %% --- 结算上一段逻辑 ---
    StitchCheck -- No (是有效空窗) --> MarkCheck{"d.pendingMark > GlitchThreshold?<br>(且 > 0)"}

    %% 信号去抖: 极短的Mark被丢弃
    MarkCheck -- No (是噪声Mark) --> GapAnalysis
    MarkCheck -- Yes (是有效Mark) --> AddMark[<b>入库 Mark</b><br>Update WPM<br>Buffer.append-pendingMark-]
    AddMark --> GapAnalysis

    %% --- Gap 性质判定 ---
    GapAnalysis{"d.lastGap > CharThreshold?<br>(> 2.5t)"}

    %% 情况 A: 字符内间隔
    GapAnalysis -- No (Intra-Char Gap) --> AddGap[<b>入库 Gap</b><br>Buffer.append-astGap]
    AddGap --> ResetState

    %% 情况 B: 字符间间隔 (触发解码)
    GapAnalysis -- Yes (Char Space) --> BufferCheck{Buffer not empty?}
    BufferCheck -- No --> ResetState
    BufferCheck -- Yes --> Normalize[<b>归一化</b><br>Normalized = Buffer / unitTime]
    Normalize --> BeamStep[[<b>调用 BeamDecoder.Step</b>]]
    BeamStep --> ClearBuf[清空 Buffer]
    
    %% 单词间隔检查
    ClearBuf --> WordCheck{"d.lastGap > WordThreshold?<br>(> 5.0t)"}
    WordCheck -- Yes --> InjectSpace[处理单词空格]
    WordCheck -- No --> ResetState
    InjectSpace --> ResetState

    %% --- 开启新一轮 ---
    ResetState[<b>重置状态</b><br>d.pendingMark = currentMark<br>d.lastGap = 0] --> RetResult([Return BeamResult])

    %% 样式
    style Start fill:#f9f,stroke:#333
    style BeamStep fill:#ff9,stroke:#f66,stroke-width:2px
    style Stitching fill:#fcc,stroke:#f66
    style AddMark fill:#cfc,stroke:#333
    style AddGap fill:#cfc,stroke:#333