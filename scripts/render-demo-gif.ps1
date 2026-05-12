$ErrorActionPreference = "Stop"

$repo = Split-Path -Parent $PSScriptRoot
$framesDir = Join-Path $repo "assets/demo-frames"
$outGif = Join-Path $repo "assets/adotop-demo.gif"
$templatePath = Join-Path $PSScriptRoot "demo-frame-template.html"

function Export-CanvasPng {
    param(
        [string] $Chrome,
        [string] $HtmlPath,
        [string] $PngPath
    )

    $stdout = [IO.Path]::GetTempFileName()
    $stderr = [IO.Path]::GetTempFileName()
    try {
        $process = Start-Process -FilePath $Chrome -ArgumentList @("--headless", "--disable-gpu", "--dump-dom", $HtmlPath) -NoNewWindow -Wait -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
        $text = [IO.File]::ReadAllText($stdout)
        $match = [regex]::Match($text, 'data:image/png;base64,([A-Za-z0-9+/=]+)')
        if (-not $match.Success) {
            $errorText = [IO.File]::ReadAllText($stderr)
            throw "Failed to export canvas PNG from $HtmlPath`n$errorText"
        }

        [IO.File]::WriteAllBytes($PngPath, [Convert]::FromBase64String($match.Groups[1].Value))
    } finally {
        Remove-Item -LiteralPath $stdout, $stderr -Force -ErrorAction SilentlyContinue
    }
}

if (Test-Path $framesDir) {
    Remove-Item -LiteralPath $framesDir -Recurse -Force
}
New-Item -ItemType Directory -Force $framesDir | Out-Null

Push-Location $repo
try {
    $env:ADOTOP_THEME = "dark"
    $raw = go run ./cmd/adotop demo --frames
} finally {
    Pop-Location
}

$frames = ($raw -join "`n") -split "`f"
$chrome = @(
    "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
    "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
) | Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1
if (-not $chrome) {
    throw "Chrome or Edge is required to render demo frames"
}

$template = Get-Content -LiteralPath $templatePath -Raw
$exportScript = @'
;(() => {
  document.body.setAttribute("data-canvas-png", canvas.toDataURL("image/png"));
})();
'@

for ($i = 0; $i -lt $frames.Count; $i++) {
    $frame = $frames[$i].TrimEnd()
    $frameB64 = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($frame))
    $html = $template.Replace("__FRAME_B64__", $frameB64).Replace("</script>", "$exportScript</script>")
    $htmlPath = Join-Path $framesDir ("frame-{0:D2}.html" -f $i)
    $pngPath = Join-Path $framesDir ("frame-{0:D2}.png" -f $i)
    Set-Content -LiteralPath $htmlPath -Value $html -Encoding UTF8
    Export-CanvasPng -Chrome $chrome -HtmlPath $htmlPath -PngPath $pngPath
}

$palette = Join-Path $framesDir "palette.png"
& ffmpeg -y -framerate 0.8 -i (Join-Path $framesDir "frame-%02d.png") -vf "palettegen" $palette | Out-Null
& ffmpeg -y -framerate 0.8 -i (Join-Path $framesDir "frame-%02d.png") -i $palette -lavfi "paletteuse=dither=bayer:bayer_scale=3" -gifflags -offsetting-transdiff $outGif | Out-Null

Remove-Item -LiteralPath $framesDir -Recurse -Force

Write-Host "Wrote $outGif"
