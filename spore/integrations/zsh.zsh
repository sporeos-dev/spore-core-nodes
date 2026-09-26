# Copyright 2026 Matt Harrison
# SPDX-License-Identifier: Apache-2.0

# Spore terminal hint integration for zsh.
# This widget leaves zsh completion unchanged except when the current command
# line begins with `spore`.

_spore_hint_tab() {
    emulate -L zsh
    setopt extendedglob

    if [[ "$BUFFER" != [[:space:]]#spore(|\ *) ]]; then
        zle .expand-or-complete
        return
    fi

    local prefix="${BUFFER%%spore*}spore"
    if [[ "$BUFFER" == "$prefix "* ]]; then
        prefix+=" "
    fi

    local body="${BUFFER#$prefix}"
    local cursor=$(( CURSOR - ${#prefix} ))
    (( cursor < 0 )) && cursor=0

    local escaped_body="${body//\\/\\\\}"
    escaped_body="${escaped_body//\"/\\\"}"
    local hint="hint body=\"${escaped_body}\" cursor=${cursor}"
    if (( cursor == ${#body} )); then
        hint+=" eol"
    fi

    # Clear zle's display while the one-shot CLI renders the hub hint, then
    # redraw the unchanged command line for continued interaction.
    zle -I
    command spore "$hint"
    zle reset-prompt
}

zle -N _spore_hint_tab
bindkey '^I' _spore_hint_tab
