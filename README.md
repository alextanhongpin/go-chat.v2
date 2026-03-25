# go-chat.v2
Remake of [go-chat](https://github.com/alextanhongpin/go-chat).


## Flow


```
init(user, friend)

user_id = authorize(token)
is_friend(user_id, friend_id)

// Can be unique across all devices, or unique per device.
session_id = newSessionPerDevice(user_id)

// Add user to room
go: loop: redis.sadd(user_id, session_id, ttl=5)
// if unique
// redis.set(user_id, online)
defer redis.srem(user_id, session_id)
redis.publish(friend_id, online)

// Run in separate goroutine
go: for msg in redis.subscribe(user_id):
  ws.write(msg)

go: for msg in ws.read:
  // match: istyping, online, offline
  // send to friend, fire and forget
  redis.publish(friend_id, msg)

defer redis.publish(friend_id, offline)
```
