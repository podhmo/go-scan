e2e:
	@echo "Running end-to-end test for find-orphans..."
	go run . --workspace-root ../.. --primary-analysis-scope "github.com/podhmo/go-scan/examples/deps-walk,github.com/podhmo/go-scan/examples/minigo" ./... > find-orphans.out 2>&1
	@echo "Output written to examples/find-orphans/find-orphans.out"
