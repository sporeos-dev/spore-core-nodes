MODULES := spore spore-log spore-shell spore-dialog spore-witness

.DEFAULT_GOAL := check

.PHONY: check analyze setup $(MODULES)

check:
	@for m in $(MODULES); do $(MAKE) -C $$m check; done

analyze:
	@for m in $(MODULES); do $(MAKE) -C $$m analyze; done

setup:
	@$(MAKE) -C spore setup
