param(
    [Parameter(Mandatory=$true)][string]$OutputPath,
    [string]$SourcePath = (Join-Path $PSScriptRoot '..\web\public\dumpersg.png')
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
# Preserve the original PNG transparency at each Windows icon resolution.
$taskSizes = @(16,20,24,32,40,48,64,128,256)
$taskImages = @()
$taskSource = [Drawing.Image]::FromFile([IO.Path]::GetFullPath($SourcePath))
try {
    foreach ($taskSize in $taskSizes) {
        $taskBitmap = [Drawing.Bitmap]::new($taskSize,$taskSize,[Drawing.Imaging.PixelFormat]::Format32bppArgb)
        $taskGraphics = [Drawing.Graphics]::FromImage($taskBitmap)
        $taskMemory = [IO.MemoryStream]::new()
        try {
            $taskGraphics.CompositingMode = [Drawing.Drawing2D.CompositingMode]::SourceCopy
            $taskGraphics.CompositingQuality = [Drawing.Drawing2D.CompositingQuality]::HighQuality
            $taskGraphics.InterpolationMode = [Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            $taskGraphics.PixelOffsetMode = [Drawing.Drawing2D.PixelOffsetMode]::HighQuality
            $taskScale = [Math]::Min($taskSize/[double]$taskSource.Width,$taskSize/[double]$taskSource.Height)
            $taskWidth = [single]($taskSource.Width*$taskScale)
            $taskHeight = [single]($taskSource.Height*$taskScale)
            $taskRect = [Drawing.RectangleF]::new(($taskSize-$taskWidth)/2,($taskSize-$taskHeight)/2,$taskWidth,$taskHeight)
            $taskGraphics.DrawImage($taskSource,$taskRect)
            $taskBitmap.Save($taskMemory,[Drawing.Imaging.ImageFormat]::Png)
            $taskImages += ,$taskMemory.ToArray()
        } finally {
            $taskMemory.Dispose(); $taskGraphics.Dispose(); $taskBitmap.Dispose()
        }
    }
} finally { $taskSource.Dispose() }
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
