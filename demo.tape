Output demo.gif

Set Shell "zsh"
Set FontSize 20
Set Width 1600
Set Height 700
Set TypingSpeed 0.08s

# Use the freshly built binary, a plain prompt, the zsh integration, and none
# of the user's shell config
Hide
Type `d=$(mktemp -d) && go build -o $d/hmm . && export PATH="$d:$PATH" && export ZDOTDIR=$d && PROMPT='$ ' && eval "$(hmm --init zsh)" && hmm --reindex -l x >/dev/null 2>&1; clear`
Enter
Sleep 3s
Show

# Describe what you want, then pick from the matches; the choice lands on
# the next prompt
Type "hmm list open files"
Sleep 500ms
Enter
Sleep 2.5s
Enter
Sleep 2s
Ctrl+U
Sleep 500ms

Type "hmm kill process by name"
Sleep 500ms
Enter
Sleep 2s
Down@600ms 2
Sleep 1s
Enter
Sleep 2s
Ctrl+U
Sleep 500ms

# Or start the picker empty and search as you type, peek at the man page,
# then run what you chose
Type "hmm"
Sleep 500ms
Enter
Sleep 2s
Type "month calendar"
Sleep 1.5s
Down
Sleep 1s
Ctrl+O
Sleep 3s
Type "q"
Sleep 1.5s
Enter
Sleep 2s
Enter
Sleep 4s
