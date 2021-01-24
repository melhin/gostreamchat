package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-redis/redis/v8"
)

const (
	// used to track users that used chat. mainly for listing users in the /users api, in real world chat app
	// such user list should be separated into user management module.
	usersKey       = "users"
	userChannelFmt = "user:%s:channels"
	ChannelsKey    = "channels"
)

var ctx = context.Background()

type User struct {
	name            string
	channelsHandler *redis.PubSub

	stopListenerChan chan struct{}
	listening        bool

	MessageChan chan redis.Message
}

//Connect connect user to user channels on redis
func Connect(rdb *redis.Client, name string) (*User, error) {
	if _, err := rdb.SAdd(ctx, usersKey, name).Result(); err != nil {
		return nil, err
	}

	u := &User{
		name:             name,
		stopListenerChan: make(chan struct{}),
		MessageChan:      make(chan redis.Message),
	}

	if err := u.connect(rdb); err != nil {
		return nil, err
	}

	return u, nil
}

func (u *User) Subscribe(rdb *redis.Client, channel string) error {

	userChannelsKey := fmt.Sprintf(userChannelFmt, u.name)

	if rdb.SIsMember(ctx, userChannelsKey, channel).Val() {
		return nil
	}
	if err := rdb.SAdd(ctx, userChannelsKey, channel).Err(); err != nil {
		return err
	}

	return u.connect(rdb)
}

func (u *User) Unsubscribe(rdb *redis.Client, channel string) error {

	userChannelsKey := fmt.Sprintf(userChannelFmt, u.name)

	if !rdb.SIsMember(ctx, userChannelsKey, channel).Val() {
		return nil
	}
	if err := rdb.SRem(ctx, userChannelsKey, channel).Err(); err != nil {
		return err
	}

	return u.connect(rdb)
}

func (u *User) connect(rdb *redis.Client) error {

	var c []string

	c1, err := rdb.SMembers(ctx, ChannelsKey).Result()
	if err != nil {
		return err
	}
	c = append(c, c1...)

	// get all user channels (from DB) and start subscribe
	c2, err := rdb.SMembers(ctx, fmt.Sprintf(userChannelFmt, u.name)).Result()
	if err != nil {
		return err
	}
	c = append(c, c2...)

	if len(c) == 0 {
		fmt.Println("no channels to connect to for user: ", u.name)
		return nil
	}

	if u.channelsHandler != nil {
		if err := u.channelsHandler.Unsubscribe(ctx); err != nil {
			return err
		}
		if err := u.channelsHandler.Close(); err != nil {
			return err
		}
	}
	if u.listening {
		u.stopListenerChan <- struct{}{}
	}

	return u.doConnect(rdb, c...)
}

func (u *User) doConnect(rdb *redis.Client, channels ...string) error {
	// subscribe all channels in one request
	pubSub := rdb.Subscribe(ctx, channels...)
	// keep channel handler to be used in unsubscribe
	u.channelsHandler = pubSub

	// The Listener
	go func() {
		u.listening = true
		fmt.Println("starting the listener for user:", u.name, "on channels:", channels)
		for {
			select {
			case msg, ok := <-pubSub.Channel():
				if !ok {
					return
				}
				u.MessageChan <- *msg

			case <-u.stopListenerChan:
				fmt.Println("stopping the listener for user:", u.name)
				return
			}
		}
	}()
	return nil
}

func (u *User) Disconnect() error {
	if u.channelsHandler != nil {
		if err := u.channelsHandler.Unsubscribe(ctx); err != nil {
			return err
		}
		if err := u.channelsHandler.Close(); err != nil {
			return err
		}
	}
	if u.listening {
		u.stopListenerChan <- struct{}{}
	}

	close(u.MessageChan)

	return nil
}

func Chat(rdb *redis.Client, channel string, content string) error {
	return rdb.Publish(ctx, channel, content).Err()
}

func List(rdb *redis.Client) ([]string, error) {
	return rdb.SMembers(ctx, usersKey).Result()
}

func GetChannels(rdb *redis.Client, username string) ([]string, error) {

	if !rdb.SIsMember(ctx, usersKey, username).Val() {
		return nil, errors.New("user not exists")
	}

	var c []string

	c1, err := rdb.SMembers(ctx, ChannelsKey).Result()
	if err != nil {
		return nil, err
	}
	c = append(c, c1...)

	// get all user channels (from DB) and start subscribe
	c2, err := rdb.SMembers(ctx, fmt.Sprintf(userChannelFmt, username)).Result()
	if err != nil {
		return nil, err
	}
	c = append(c, c2...)

	return c, nil
}
