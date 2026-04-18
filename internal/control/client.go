package control

import (
	"context"
	"encoding/json"
	"errors"
	"net"
)

type Client struct {
	socketPath string
}

func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

func (c *Client) Do(ctx context.Context, req Request) (Response, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, err
	}

	if !resp.OK {
		return Response{}, errors.New(resp.Error)
	}

	return resp, nil
}

func (c *Client) Reload(ctx context.Context, configYAML string) error {
	_, err := c.Do(ctx, Request{Command: CommandReload, ConfigYAML: configYAML})
	return err
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	resp, err := c.Do(ctx, Request{Command: CommandStatus})
	if err != nil {
		return Status{}, err
	}
	if resp.Status == nil {
		return Status{}, errors.New("missing status payload")
	}
	return *resp.Status, nil
}

func (c *Client) Stop(ctx context.Context) error {
	_, err := c.Do(ctx, Request{Command: CommandStop})
	return err
}
