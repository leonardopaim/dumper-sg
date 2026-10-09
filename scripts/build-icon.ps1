param([Parameter(Mandatory=$true)][string]$OutputPath)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
# Reproduce the existing SVG geometry at each Windows icon resolution.
$taskSizes = @(16,32,48,64,128,256)
$taskImages = @()
foreach ($taskSize in $taskSizes) {
    $taskBitmap = [Drawing.Bitmap]::new($taskSize,$taskSize)
    $taskGraphics = [Drawing.Graphics]::FromImage($taskBitmap)
    $taskGraphics.SmoothingMode = [Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $taskGraphics.ScaleTransform($taskSize/64.0,$taskSize/64.0)
    $taskPath = [Drawing.Drawing2D.GraphicsPath]::new()
    $taskPath.AddArc(0,0,30,30,180,90)
    $taskPath.AddArc(34,0,30,30,270,90)
    $taskPath.AddArc(34,34,30,30,0,90)
    $taskPath.AddArc(0,34,30,30,90,90)
    $taskPath.CloseFigure()
    $taskBrush = [Drawing.Drawing2D.LinearGradientBrush]::new([Drawing.Rectangle]::new(0,0,64,64),[Drawing.ColorTranslator]::FromHtml('#6387ff'),[Drawing.ColorTranslator]::FromHtml('#365bd9'),45.0)
    $taskGraphics.FillPath($taskBrush,$taskPath)
    $taskPen = [Drawing.Pen]::new([Drawing.Color]::White,3.5)
    $taskPen.StartCap = [Drawing.Drawing2D.LineCap]::Round
    $taskPen.EndCap = [Drawing.Drawing2D.LineCap]::Round
    $taskPen.LineJoin = [Drawing.Drawing2D.LineJoin]::Round
    $taskGraphics.DrawEllipse($taskPen,13,12,28,12)
    $taskGraphics.DrawLine($taskPen,13,18,13,42)
    $taskGraphics.DrawArc($taskPen,13,36,28,12,0,180)
    $taskGraphics.DrawArc($taskPen,13,24,28,12,0,180)
    $taskGraphics.DrawLine($taskPen,41,18,41,26)
    $taskGraphics.DrawLine($taskPen,46,33,46,52)
    $taskGraphics.DrawLine($taskPen,39,45,46,52)
    $taskGraphics.DrawLine($taskPen,46,52,53,45)
    $taskMemory = [IO.MemoryStream]::new()
    $taskBitmap.Save($taskMemory,[Drawing.Imaging.ImageFormat]::Png)
    $taskImages += ,$taskMemory.ToArray()
    $taskMemory.Dispose(); $taskPen.Dispose(); $taskBrush.Dispose(); $taskPath.Dispose(); $taskGraphics.Dispose(); $taskBitmap.Dispose()
}
$taskStream = [IO.File]::Create([IO.Path]::GetFullPath($OutputPath))
$taskWriter = [IO.BinaryWriter]::new($taskStream)
try {
    $taskWriter.Write([uint16]0); $taskWriter.Write([uint16]1); $taskWriter.Write([uint16]$taskSizes.Count)
    $taskOffset = 6 + 16*$taskSizes.Count
    for ($taskIndex=0; $taskIndex -lt $taskSizes.Count; $taskIndex++) {
        $taskDimension = if ($taskSizes[$taskIndex] -eq 256) { 0 } else { $taskSizes[$taskIndex] }
        $taskWriter.Write([byte]$taskDimension); $taskWriter.Write([byte]$taskDimension)
        $taskWriter.Write([byte]0); $taskWriter.Write([byte]0)
        $taskWriter.Write([uint16]1); $taskWriter.Write([uint16]32)
        $taskWriter.Write([uint32]$taskImages[$taskIndex].Length); $taskWriter.Write([uint32]$taskOffset)
        $taskOffset += $taskImages[$taskIndex].Length
    }
    foreach ($taskImage in $taskImages) { $taskWriter.Write([byte[]]$taskImage) }
} finally { $taskWriter.Dispose(); $taskStream.Dispose() }
