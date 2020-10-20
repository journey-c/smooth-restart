# 长连接平滑重启demo
```

---------------------------------------------------------------------------------------------
					 old process                         new process
send upgrade signal ---> |                                    |
			 |    ----------- fork ----------->   |
			 |                                    | start listen and init finish
			 |    <- send init finish signal --   |
	 close listening |                                    |
			 |    send connection state data ->   |
			 |                                    | accept and init finish
			 |    <---- send finish signal ----   |
		    exit |                                    |
---------------------------------------------------------------------------------------------
Use a domain socket to transfer connection state data

```
