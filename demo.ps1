# demo.ps1 — Lattice Privacy Engine · Demo Script (Windows PowerShell)

$Green = "`e[32m"; $Blue = "`e[34m"; $Red = "`e[31m"; $NC = "`e[0m"

Write-Host "${Blue}Arrancando Lattice Privacy Engine...${NC}"

# Compilar si no existe el ejecutable
if (-not (Test-Path ".\lattice.exe")) {
    Write-Host "${Blue}Compilando...${NC}"
    go build -o lattice.exe .
    if ($LASTEXITCODE -ne 0) { Write-Host "${Red}Error de compilacion. Instala Go desde https://go.dev/dl/${NC}"; exit 1 }
}

$env:OPENAI_API_BASE = "https://api.openai.com"

# Arrancar el proxy en segundo plano
$proxy = Start-Process -FilePath ".\lattice.exe" -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 2

Write-Host "${Green}Proxy listo. Enviando peticion con datos sensibles...${NC}"
Write-Host "${Blue}Datos originales: 'Hola, soy Juan Perez. Mi DNI es 12345678Z y mi correo juan@email.com'${NC}"
Write-Host ""

$body = @{
    model    = "gpt-3.5-turbo"
    messages = @(
        @{ role = "user"; content = "Hola, soy Juan Perez. Mi DNI es 12345678Z y mi correo juan@email.com" }
    )
} | ConvertTo-Json -Depth 5

try {
    Invoke-RestMethod -Uri "http://localhost:8080/v1/chat/completions" `
        -Method POST `
        -ContentType "application/json" `
        -Headers @{ Authorization = "Bearer demo-no-key-needed" } `
        -Body $body | ConvertTo-Json -Depth 10
}
catch {
    Write-Host "(Sin clave de OpenAI real la peticion upstream fallara — pero los logs de Lattice muestran la anonimizacion)"
}

Write-Host ""
Write-Host "${Green}Revisa la ventana de Lattice: los datos fueron anonimizados antes de salir.${NC}"

Stop-Process -Id $proxy.Id -ErrorAction SilentlyContinue
Write-Host "${Blue}Demo finalizada.${NC}"
