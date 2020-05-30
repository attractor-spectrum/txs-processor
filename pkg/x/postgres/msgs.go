package postgres

import (
	"context"
	"fmt"
	"time"

	watcher "github.com/mapofzones/cosmos-watcher/pkg/types"
	processor "github.com/mapofzones/txs-processor/pkg/types"
)

func (p *PostgresProcessor) handleTransaction(ctx context.Context, metadata processor.MessageMetadata, msg watcher.Transaction) error {
	// this should not happen
	if metadata.TxMetadata == nil {
		panic(fmt.Errorf("%w: could not fetch tx metadata", processor.CommitError))
	}

	// if tx had errors and did not affect the state
	if !metadata.TxMetadata.Accepted {
		return nil
	}

	if p.txStats == nil {
		p.txStats = &processor.TxStats{
			ChainID: metadata.ChainID,
			Hour:    metadata.BlockTime.Truncate(time.Hour),
		}
	}

	hasIBCTransfers := false
	// process each tx message
	for _, m := range msg.Messages {
		if _, ok := m.(watcher.IBCTransfer); ok {
			hasIBCTransfers = true
		}
		handle := p.Handler(m)
		if handle != nil {
			err := handle(ctx, metadata, m)
			if err != nil {
				return err
			}
		}
	}

	// increment tx stats
	p.txStats.Count++
	// if tx had ibc transfers, mark it
	if hasIBCTransfers {
		p.txStats.TxWithIBCTransfer++
	}

	return nil
}

func (p *PostgresProcessor) handleCreateClient(ctx context.Context, metadata processor.MessageMetadata, msg watcher.CreateClient) error {
	p.clients[msg.ClientID] = msg.ChainID
	return nil
}

func (p *PostgresProcessor) handleCreateConnection(ctx context.Context, metadata processor.MessageMetadata, msg watcher.CreateConnection) error {
	p.connections[msg.ConnectionID] = msg.ClientID
	return nil
}

func (p *PostgresProcessor) handleCreateChannel(ctx context.Context, metadata processor.MessageMetadata, msg watcher.CreateChannel) error {
	p.channels[msg.ChannelID] = msg.ConnectionID
	return nil
}

func (p *PostgresProcessor) handleOpenChannel(ctx context.Context, metadata processor.MessageMetadata, msg watcher.OpenChannel) error {
	p.channelStates[msg.ChannelID] = true
	return nil
}

func (p *PostgresProcessor) handleCloseChannel(ctx context.Context, metadata processor.MessageMetadata, msg watcher.CloseChannel) error {
	p.channelStates[msg.ChannelID] = false
	return nil
}

func (p *PostgresProcessor) handleIBCTransfer(ctx context.Context, metadata processor.MessageMetadata, msg watcher.IBCTransfer) error {
	chainID, err := p.ChainID(ctx, msg.ChannelID, metadata.ChainID)
	if err != nil {
		return fmt.Errorf("%w: %s", processor.ConnectionError, err.Error())
	}
	if chainID == "" {
		return fmt.Errorf("%w: could not fetch chainID connected to given channelID", processor.CommitError)
	}

	if msg.Source {
		p.ibcStats.Append(metadata.ChainID, chainID, metadata.BlockTime)
	} else {
		p.ibcStats.Append(chainID, metadata.ChainID, metadata.BlockTime)
	}

	return nil
}
