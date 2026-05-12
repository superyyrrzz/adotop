$ErrorActionPreference = "Stop"

$repo = Split-Path -Parent $PSScriptRoot
$framesDir = Join-Path $repo "assets/demo-frames"
$outGif = Join-Path $repo "assets/adotop-demo.gif"

function Convert-AnsiToHtml([string]$ansi) {
    $script:fg = $null
    $script:bg = $null
    $script:bold = $false
    $script:out = New-Object System.Text.StringBuilder
    $pattern = [regex]"\x1b\[([0-9;]*)m"
    $pos = 0

    function Append-Escaped([string]$s) {
        if ($s.Length -eq 0) { return }
        $encoded = [System.Net.WebUtility]::HtmlEncode($s)
        if ($script:fg -or $script:bg -or $script:bold) {
            $styles = @()
            if ($script:fg) { $styles += "color:$script:fg" }
            if ($script:bg) { $styles += "background-color:$script:bg" }
            if ($script:bold) { $styles += "font-weight:700" }
            [void]$script:out.Append('<span style="' + ($styles -join ';') + '">' + $encoded + '</span>')
        } else {
            [void]$script:out.Append($encoded)
        }
    }

    function Ansi256([int]$n) {
        $basic = @(
            '#000000', '#cd3131', '#0dbc79', '#e5e510', '#2472c8', '#bc3fbc', '#11a8cd', '#e5e5e5',
            '#666666', '#f14c4c', '#23d18b', '#f5f543', '#3b8eea', '#d670d6', '#29b8db', '#ffffff'
        )
        if ($n -lt 16) { return $basic[$n] }
        if ($n -ge 16 -and $n -le 231) {
            $v = $n - 16
            $r = [math]::Floor($v / 36)
            $g = [math]::Floor(($v % 36) / 6)
            $b = $v % 6
            $steps = @(0, 95, 135, 175, 215, 255)
            return ('#{0:x2}{1:x2}{2:x2}' -f $steps[$r], $steps[$g], $steps[$b])
        }
        $level = 8 + (($n - 232) * 10)
        return ('#{0:x2}{0:x2}{0:x2}' -f $level)
    }

    function Apply-Code([int[]]$codes) {
        if ($codes.Count -eq 0) { $codes = @(0) }
        for ($i = 0; $i -lt $codes.Count; $i++) {
            $c = $codes[$i]
            switch ($c) {
                0 { $script:fg = $null; $script:bg = $null; $script:bold = $false }
                1 { $script:bold = $true }
                22 { $script:bold = $false }
                39 { $script:fg = $null }
                49 { $script:bg = $null }
                { $_ -ge 30 -and $_ -le 37 } { $script:fg = Ansi256($c - 30) }
                { $_ -ge 90 -and $_ -le 97 } { $script:fg = Ansi256($c - 90 + 8) }
                { $_ -ge 40 -and $_ -le 47 } { $script:bg = Ansi256($c - 40) }
                { $_ -ge 100 -and $_ -le 107 } { $script:bg = Ansi256($c - 100 + 8) }
                38 {
                    if ($i + 2 -lt $codes.Count -and $codes[$i + 1] -eq 5) {
                        $script:fg = Ansi256($codes[$i + 2]); $i += 2
                    } elseif ($i + 4 -lt $codes.Count -and $codes[$i + 1] -eq 2) {
                        $script:fg = ('#{0:x2}{1:x2}{2:x2}' -f $codes[$i + 2], $codes[$i + 3], $codes[$i + 4]); $i += 4
                    }
                }
                48 {
                    if ($i + 2 -lt $codes.Count -and $codes[$i + 1] -eq 5) {
                        $script:bg = Ansi256($codes[$i + 2]); $i += 2
                    } elseif ($i + 4 -lt $codes.Count -and $codes[$i + 1] -eq 2) {
                        $script:bg = ('#{0:x2}{1:x2}{2:x2}' -f $codes[$i + 2], $codes[$i + 3], $codes[$i + 4]); $i += 4
                    }
                }
            }
        }
    }

    foreach ($m in $pattern.Matches($ansi)) {
        Append-Escaped $ansi.Substring($pos, $m.Index - $pos)
        $codes = @()
        if ($m.Groups[1].Value -ne '') {
            $codes = $m.Groups[1].Value.Split(';') | ForEach-Object { [int]$_ }
        }
        Apply-Code $codes
        $pos = $m.Index + $m.Length
    }
    Append-Escaped $ansi.Substring($pos)
    return $script:out.ToString()
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

for ($i = 0; $i -lt $frames.Count; $i++) {
    $htmlFrame = Convert-AnsiToHtml $frames[$i].TrimEnd()
    $html = @"
<!doctype html>
<html>
<head>
<meta charset="utf-8">
<style>
html, body { margin: 0; width: 1200px; height: 820px; background: #0b0f16; }
body { display: flex; align-items: center; justify-content: center; }
.terminal {
  width: 1140px;
  height: 760px;
  box-sizing: border-box;
  padding: 24px 30px;
  background: #11111b;
  color: #cdd6f4;
  border: 1px solid #45475a;
  border-radius: 10px;
  box-shadow: 0 20px 50px rgba(0,0,0,.38);
  overflow: hidden;
}
pre {
  margin: 0;
  font: 14px/1.25 Consolas, "Cascadia Mono", "SFMono-Regular", monospace;
  white-space: pre;
}
span { display: inline; }
</style>
</head>
<body><div class="terminal"><pre>$htmlFrame</pre></div></body>
</html>
"@
    $htmlPath = Join-Path $framesDir ("frame-{0:D2}.html" -f $i)
    $pngPath = Join-Path $framesDir ("frame-{0:D2}.png" -f $i)
    Set-Content -LiteralPath $htmlPath -Value $html -Encoding UTF8
    & $chrome --headless --disable-gpu --hide-scrollbars --window-size=1200,820 --screenshot="$pngPath" $htmlPath | Out-Null
}

$palette = Join-Path $framesDir "palette.png"
& ffmpeg -y -framerate 0.8 -i (Join-Path $framesDir "frame-%02d.png") -vf "palettegen" $palette | Out-Null
& ffmpeg -y -framerate 0.8 -i (Join-Path $framesDir "frame-%02d.png") -i $palette -lavfi "paletteuse=dither=bayer:bayer_scale=3" $outGif | Out-Null

Remove-Item -LiteralPath $framesDir -Recurse -Force

Write-Host "Wrote $outGif"
