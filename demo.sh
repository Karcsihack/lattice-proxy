#!/bin/bash

GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}🚀 Arrancando Lattice Privacy Engine...${NC}"

# Compilar si no existe el binario
if [ ! -f "./lattice" ]; then
  echo -e "${BLUE}⚙️  Compilando...${NC}"
  go build -o lattice . || { echo -e "${RED}❌ Error de compilación. ¿Tienes Go instalado?${NC}"; exit 1; }
fi

# Arrancar el proxy en segundo plano
OPENAI_API_BASE=https://api.openai.com ./lattice &
PROXY_PID=$!

# Esperar a que el servidor esté listo
sleep 2

echo -e "${GREEN}✅ Proxy listo. Enviando petición con datos sensibles...${NC}"
echo -e "${BLUE}Datos originales: 'Hola, soy Juan Pérez. Mi DNI es 12345678Z y mi correo juan@email.com'${NC}\n"

curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer demo-no-key-needed-for-local-test" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {"role": "user", "content": "Hola, soy Juan Pérez. Mi DNI es 12345678Z y mi correo juan@email.com"}
    ]
  }'

echo -e "\n\n${GREEN}⬆️  Revisa los logs de arriba. ¡Los datos han sido anonimizados antes de salir!${NC}"

kill $PROXY_PID 2>/dev/null
echo -e "${BLUE}Demo finalizada.${NC}"
