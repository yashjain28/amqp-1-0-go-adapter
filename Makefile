# Go parameters
	ADAPTER_NAME=$(name)
    GOCMD=go
    GOBUILD=$(GOCMD) build
    GOCLEAN=$(GOCMD) clean
    GOTEST=$(GOCMD) test
    GOGET=$(GOCMD) get
    LINUX_AMD64=$(ADAPTER_NAME)_linux_amd64
    LINUX_ARMv7=$(ADAPTER_NAME)_linux_armv7
    
    all: clean deps test build-linux-amd64 build-linux-armv7
    build: 
		$(GOBUILD) -o $(LINUX_AMD64) -v
    test: 
		$(GOTEST) -v ./...
    clean: 
		$(GOCLEAN)
		rm -f $(LINUX_AMD64)
		rm -f $(LINUX_ARMv7)
    run:
		$(GOBUILD) -o $(LINUX_AMD64) -v ./...
		./$(LINUX_AMD64)
    deps:
		$(GOGET)
		$(GOGET) github.com/clearblade/Go-SDK
		$(GOGET) github.com/clearblade/adapter-go-library
		$(GOGET) github.com/clearblade/mqtt_parsing
		
    
    
    build-linux-amd64:
		GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(LINUX_AMD64) -v

    build-linux-armv7:
		GOOS=linux GOARCH=armv7 $(GOBUILD) -o $(LINUX_AMD64) -v
    