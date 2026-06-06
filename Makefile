MODULES := spore spore-log spore-shell spore-witness

.DEFAULT_GOAL := check

.PHONY: check analyze setup $(MODULES)

check:
	@$(MAKE) -C spore check
	@$(MAKE) -C spore-log check
	@$(MAKE) -C spore-shell check
	@$(MAKE) -C spore-witness check

analyze:
	@$(MAKE) -C spore analyze
	@$(MAKE) -C spore-log analyze
	@$(MAKE) -C spore-shell analyze
	@$(MAKE) -C spore-witness analyze

setup:
	@$(MAKE) -C spore setup
