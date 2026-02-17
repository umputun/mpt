# fish completion for mpt (generated via go-flags)
complete -c mpt -a '(GO_FLAGS_COMPLETION=verbose mpt (commandline -cop) 2>/dev/null | string replace -r "\\s+# " "\t")'
