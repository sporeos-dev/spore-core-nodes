# Copyright 2026 Matt Harrison
# SPDX-License-Identifier: Apache-2.0

if (-not (Get-Command Set-PSReadLineKeyHandler -ErrorAction SilentlyContinue)) {
    return
}

$script:sporeHintExecutable = Join-Path (Split-Path -Parent $PSScriptRoot) 'store\spore\spore.exe'

Set-PSReadLineKeyHandler -Key Tab -ScriptBlock {
    $line = $null
    $cursor = 0
    [Microsoft.PowerShell.PSConsoleReadLine]::GetBufferState([ref]$line, [ref]$cursor)

    $match = [regex]::Match($line, '^\s*spore(?=\s|$)')
    if (-not $match.Success) {
        [Microsoft.PowerShell.PSConsoleReadLine]::TabCompleteNext()
        return
    }

    $bodyOffset = $match.Length
    if ($line.Length -gt $bodyOffset -and [char]::IsWhiteSpace($line[$bodyOffset])) {
        $bodyOffset++
    }
    $body = $line.Substring($bodyOffset)
    $bodyCursor = [Math]::Max(0, [Math]::Min($body.Length, $cursor - $bodyOffset))

    $hintCursor = 0
    for ($i = 0; $i -lt $bodyCursor; $i++) {
        if ([char]::IsHighSurrogate($body[$i]) -and
            $i + 1 -lt $bodyCursor -and
            [char]::IsLowSurrogate($body[$i + 1])) {
            $i++
        }
        $hintCursor++
    }

    $escapedBody = $body.Replace('\', '\\').Replace('"', '\"')
    $hint = "hint body=`"$escapedBody`" cursor=$hintCursor"
    if ($bodyCursor -eq $body.Length) {
        $hint += ' eol'
    }

    [Microsoft.PowerShell.PSConsoleReadLine]::SetCursorPosition($line.Length)
    [Console]::WriteLine()
    Start-Process -FilePath $script:sporeHintExecutable -ArgumentList $hint -NoNewWindow -Wait
    $promptY = [Console]::CursorTop
    [Microsoft.PowerShell.PSConsoleReadLine]::InvokePrompt($null, $promptY)
    [Microsoft.PowerShell.PSConsoleReadLine]::SetCursorPosition($cursor)
}
