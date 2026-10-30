$ErrorActionPreference = 'Stop'
$consolePython = Join-Path $PSScriptRoot '.venv\Scripts\pythonw.exe'
$consoleEntry = Join-Path $PSScriptRoot 'app.py'
if (-not (Test-Path -LiteralPath $consolePython)) {
    throw '未找到控制台虚拟环境，请按 README.md 重新创建。'
}
Start-Process -FilePath $consolePython -ArgumentList ('"' + $consoleEntry + '"') -WorkingDirectory $PSScriptRoot -WindowStyle Normal
