# Root Makefile - delegates all targets to entrypoint/Makefile

%:
	$(MAKE) -C entrypoint $@
