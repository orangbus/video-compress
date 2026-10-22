<template>
  <div>
    <h2 class="page-title">说明</h2>
    <p class="page-sub">视频批量压缩工具（H.265 / HEVC）使用说明</p>

    <div class="card help-section">
      <h3>一、它能做什么</h3>
      <p>把一批视频批量压缩为 <b>H.265 (HEVC)</b> 编码，保留原始分辨率和帧率，带实时进度。<br />
        默认 CRF 18 已是画质很好的档位，适合讲课录屏、网课等场景，体积通常可减小 50%~70%。</p>

      <h3>二、快速上手</h3>
      <ul>
        <li>① 「开始」页查看环境检测，缺 ffmpeg 时点击「自动下载」即可（下载到软件当前目录）。</li>
        <li>② 选择压缩模式（CPU 软编 / GPU 硬编 / 更清晰画质），也可以展开「高级选项」自定义参数。</li>
        <li>③ 拖拽或选择视频文件/文件夹，设置并行数，点击「开始压缩」。</li>
        <li>④ 在「转化」页查看实时进度，在「完成」页查看转化时长、压缩比与保存位置。</li>
      </ul>

      <h3>三、两种编码器怎么选</h3>
      <table>
        <thead>
          <tr><th>对比</th><th>libx265（CPU 软编）</th><th>nvenc（GPU 硬编）</th></tr>
        </thead>
        <tbody>
          <tr><td>速度</td><td>慢（占满 CPU）</td><td>快 5~10 倍（用显卡）</td></tr>
          <tr><td>同画质下体积</td><td>更小</td><td>略大</td></tr>
          <tr><td>适用场景</td><td>追求极限压缩比、不赶时间</td><td>批量大视频、追求速度</td></tr>
        </tbody>
      </table>
      <p style="margin-top: 8px"><b>建议</b>：讲课、录屏这类画面静止多的视频，硬编画质差别几乎看不出来，直接选「GPU 硬编」。</p>

      <h3>四、CRF / QP 画质档位</h3>
      <table>
        <thead>
          <tr><th>档位</th><th>说明</th></tr>
        </thead>
        <tbody>
          <tr><td>CRF 18（CPU 软编默认）</td><td>画质最好，体积约原片的 35%~40%</td></tr>
          <tr><td>CRF 23（CPU 软编推荐）</td><td>视觉几乎无差别，体积约原片的 25%，性价比最高</td></tr>
          <tr><td>QP 28（GPU 硬编默认）</td><td>速度最快，体积约原片的 60%~70%</td></tr>
          <tr><td>QP 26（GPU 更清晰）</td><td>画质更清晰，体积约原片的 80% 左右</td></tr>
        </tbody>
      </table>
      <p style="margin-top: 8px">CPU 软编用 CRF（每 +6 体积大致减半）；GPU 硬编用 QP（值与 CRF 不可直接换算，实测 QP 24≈原大小、QP 28≈70%、QP 32≈50%）。想更小：调高数值 → 换更慢的 preset → 降分辨率（会损失清晰度，不建议）。</p>

      <h3>五、出错与断点续传</h3>
      <ul>
        <li>批量压缩中某个文件出错<b>不会中断</b>其它任务，错误原因会显示在任务列表。</li>
        <li>失败任务可在「转化」页一键「重试失败」。</li>
        <li>已经压缩完成的文件（输出已存在）再次运行时<b>自动跳过</b>，不会重复压缩。</li>
        <li>输出目录默认在源视频所在文件夹的 <code>compressed</code> 子目录，<b>不会覆盖原文件</b>。</li>
      </ul>

      <h3>六、常见问题</h3>
      <ul>
        <li><b>GPU 硬编报错？</b> 显卡不支持 HEVC 硬编（需 NVIDIA GTX 10 系及以上），改用「CPU 软编」。</li>
        <li><b>压缩后文件反而变大？</b> 若源文件已是高压缩率编码（如 H.265），或内容为高噪声画面，重编码可能不缩小，属正常现象。</li>
        <li><b>输出文件打不开 / iPhone 不兼容？</b> 输出 mp4/mov 已自动加 <code>hvc1</code> 标签，兼容 iPhone、QuickTime。</li>
      </ul>
    </div>
  </div>
</template>
