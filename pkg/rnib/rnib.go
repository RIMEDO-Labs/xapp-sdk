package rnib

import (
	"context"

	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	toposdk "github.com/onosproject/onos-ric-sdk-go/pkg/topo"
)

var log = logging.GetLogger("rnib")

type TopoClient interface {
	WatchE2Connections(ctx context.Context, ch chan topoapi.Event) error
}

type Options struct {
	TopoAddress string
	TopoPort    int
}

type Cell struct {
	CGI      string
	CellType string
}

func NewClient(options Options) (Client, error) {
	sdkClient, err := toposdk.NewClient(
		toposdk.WithTopoAddress(
			options.TopoAddress,
			options.TopoPort,
		),
	)
	if err != nil {
		return Client{}, err
	}
	return Client{
		client: sdkClient,
	}, nil
}

type Client struct {
	client toposdk.Client
}

func getControlRelationFilter() *topoapi.Filters {
	controlRelationFilter := &topoapi.Filters{
		KindFilter: &topoapi.Filter{
			Filter: &topoapi.Filter_Equal_{
				Equal_: &topoapi.EqualFilter{
					Value: topoapi.CONTROLS,
				},
			},
		},
	}
	return controlRelationFilter
}

func (c *Client) WatchE2Connections(ctx context.Context, ch chan topoapi.Event) error {
	err := c.client.Watch(ctx, ch, toposdk.WithWatchFilters(getControlRelationFilter()))
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) GetE2CellFilter() *topoapi.Filters {
	cellEntityFilter := &topoapi.Filters{
		KindFilter: &topoapi.Filter{
			Filter: &topoapi.Filter_In{
				In: &topoapi.InFilter{
					Values: []string{topoapi.E2CELL},
				},
			},
		},
	}
	return cellEntityFilter
}

func (c *Client) GetCellTypes(ctx context.Context) (map[string]Cell, error) {
	output := make(map[string]Cell)

	cells, err := c.client.List(ctx, toposdk.WithListFilters(c.GetE2CellFilter()))
	if err != nil {
		log.Warn(err)
		return output, err
	}

	for _, cell := range cells {

		cellObject := &topoapi.E2Cell{}
		err = cell.GetAspect(cellObject)
		if err != nil {
			log.Warn(err)
		}
		output[string(cell.ID)] = Cell{
			CGI:      cellObject.CellObjectID,
			CellType: cellObject.CellType,
		}
	}
	return output, nil
}

func (c *Client) SetCellType(ctx context.Context, id string, cellType string) error {
	cell, err := c.client.Get(ctx, topoapi.ID(id))
	if err != nil {
		log.Warn(err)
		return err
	}

	cellObject := &topoapi.E2Cell{}
	err = cell.GetAspect(cellObject)
	if err != nil {
		log.Warn(err)
		return err
	}

	cellObject.CellType = cellType

	err = cell.SetAspect(cellObject)
	if err != nil {
		log.Warn(err)
		return err
	}
	err = c.client.Update(ctx, cell)
	if err != nil {
		log.Warn(err)
		return err
	}

	return nil
}
