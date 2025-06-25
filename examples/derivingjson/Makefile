# Makefile for regenerating derivingjson testdata

# Define the directories containing the models
TESTDATA_DIRS := testdata/simple testdata/separated/models

# Define a target for each directory
TARGET_FILES := $(foreach dir,$(TESTDATA_DIRS),$(dir)/%_deriving.go)

# Default target to update all testdata
.PHONY: all
all: update-testdata

# Target to update all _deriving.go files
.PHONY: update-testdata
update-testdata: $(TARGET_FILES)

# Rule to update _deriving.go files
# For each directory, run the generator
# The $< refers to the first prerequisite (the directory itself in this pattern rule)
# The $* refers to the stem of the target pattern (e.g., "simple" or "separated/models")
%/models_deriving.go: %/models.go
	@echo "Generating for $* (models)"
	@go run ./*.go ./$(*)

%/simple_deriving.go: %/models.go
	@echo "Generating for $* (simple)"
	@go run ./*.go ./$(*)

# A more generic single rule attempt, might need refinement based on exact _deriving.go naming
# If all _deriving.go files are named based on their directory, this could work.
# However, the current files are models_deriving.go and simple_deriving.go which don't directly match their directory names.
# The above explicit rules are safer.

# Phony target for cleaning generated files (optional, but good practice)
.PHONY: clean
clean:
	@echo "Cleaning generated files..."
	@rm -f $(TESTDATA_DIRS:=/%_deriving.go)
	@rm -f testdata/simple/simple_deriving.go
	@rm -f testdata/separated/models/models_deriving.go

# Explicit targets for each generation to ensure they run
# This is more robust than relying on wildcard pattern matching if file structures are very specific.
update-simple:
	@echo "Generating for testdata/simple"
	@go run ./*.go ./testdata/simple

update-separated-models:
	@echo "Generating for testdata/separated/models"
	@go run ./*.go ./testdata/separated/models

# Consolidate update-testdata to call specific targets
update-testdata-explicit: update-simple update-separated-models
	@echo "All testdata updated."

# Make 'all' use the explicit update target
all: update-testdata-explicit

# Add a help target
.PHONY: help
help:
	@echo "Makefile for regenerating derivingjson testdata"
	@echo ""
	@echo "Targets:"
	@echo "  all                     - Update all testdata files (default)"
	@echo "  update-testdata         - Alias for 'all'"
	@echo "  update-simple           - Update testdata in testdata/simple"
	@echo "  update-separated-models - Update testdata in testdata/separated/models"
	@echo "  clean                   - Remove generated _deriving.go files"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Example:"
	@echo "  make all"
	@echo "  make clean"

# Specify Go files for the generator. This makes the dependency explicit.
GENERATOR_FILES = $(wildcard ./*.go)

# Update rules to depend on generator files
testdata/simple/simple_deriving.go: testdata/simple/models.go $(GENERATOR_FILES)
	@echo "Generating for testdata/simple"
	@go run $(GENERATOR_FILES) ./testdata/simple

testdata/separated/models/models_deriving.go: testdata/separated/models/models.go $(GENERATOR_FILES)
	@echo "Generating for testdata/separated/models"
	@go run $(GENERATOR_FILES) ./testdata/separated/models

# Update the consolidated target
update-testdata: testdata/simple/simple_deriving.go testdata/separated/models/models_deriving.go
	@echo "All testdata updated."

# Default target
all: update-testdata

# Clean target remains useful
clean:
	@echo "Cleaning generated files..."
	@rm -f testdata/simple/simple_deriving.go
	@rm -f testdata/separated/models/models_deriving.go

.PHONY: all update-testdata clean help testdata/simple/simple_deriving.go testdata/separated/models/models_deriving.go
