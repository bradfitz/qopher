run:
	$(HOME)/sdk/appengine/dev_appserver.py --skip_sdk_update_check=yes $(GOPATH)/src/qopher/appengine

wipe:
	$(HOME)/sdk/appengine/dev_appserver.py --skip_sdk_update_check=yes --clear_datastore=yes $(GOPATH)/src/qopher/appengine

test:
	go test -v qopher/task/...

deploy:
	$(HOME)/sdk/appengine/appcfg.py update --oauth2 $(GOPATH)/src/qopher/appengine

