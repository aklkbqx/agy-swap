#!/bin/bash
set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}Installing agy-swap...${NC}"

if ! command -v python3 &> /dev/null; then
    echo -e "${RED}Error: python3 is not installed. Please install Python 3 first.${NC}"
    exit 1
fi

TARGET_DIR="$HOME/.local/bin"
mkdir -p "$TARGET_DIR"

if [ -f "agy-swap" ]; then
    cp agy-swap "$TARGET_DIR/agy-swap"
else
    echo -e "${BLUE}Downloading agy-swap from GitHub...${NC}"
    curl -fsSL -o "$TARGET_DIR/agy-swap" https://raw.githubusercontent.com/aklkbqx/agy-swap/main/agy-swap
fi

chmod +x "$TARGET_DIR/agy-swap"

echo -e "${GREEN}✔ agy-swap successfully installed to $TARGET_DIR/agy-swap${NC}"

if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
    echo -e "\n${YELLOW}⚠ Note: $HOME/.local/bin is not in your PATH.${NC}"
    echo -e "To run 'agy-swap' globally from anywhere, add it to your shell configuration:"
    
    SHELL_RC=""
    if [[ "$SHELL" == */zsh ]]; then
        SHELL_RC="$HOME/.zshrc"
    elif [[ "$SHELL" == */bash ]]; then
        SHELL_RC="$HOME/.bashrc"
    fi
    
    if [ -n "$SHELL_RC" ]; then
        echo -e "Run the following command to add it automatically:"
        echo -e "  ${BLUE}echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> $SHELL_RC && source $SHELL_RC${NC}"
    else
        echo -e "  ${BLUE}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
    fi
else
    echo -e "\n${GREEN}🎉 Installation complete! You can now run 'agy-swap' from anywhere.${NC}"
fi
