run:
	$(HOME)/sdk/appengine/dev_appserver.py $(GOPATH)/src/qopher/appengine

wipe:
	$(HOME)/sdk/appengine/dev_appserver.py --clear_datastore=yes $(GOPATH)/src/qopher/appengine

test:
	go test -v qopher/task/...

deploy:
	$(HOME)/sdk/appengine/appcfg.py update --oauth2 $(GOPATH)/src/qopher/appengine

