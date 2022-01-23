package rnib

import (
	"context"

	topoAPI "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	topoSDK "github.com/onosproject/onos-ric-sdk-go/pkg/topo"
)

var log = logging.GetLogger("R-NIB client source file [pkg/rnib/rnib.go]")

type TopologyClient interface {
}

type Client struct {
	client topoSDK.Client
}

func NewClient() (Client, error) {

	newClient, err := topoSDK.NewClient()
	if err != nil {

		log.Warn("Problem occured during creating new client object [NewClient()].")
		return Client{}, err

	}

	client := Client{

		client: newClient,
	}

	return client, nil

}

type Cell struct {
	CGI      string
	CellType string
}

func (self *Client) GetE2NodeIDs(ctx context.Context) ([]topoAPI.ID, error) {

	objects, err := self.client.List(ctx, topoSDK.WithListFilters(getControlRelationFilter()))
	if err != nil {
		return nil, err
	}

	e2NodeIDs := make([]topoAPI.ID, len(objects))

	for _, object := range objects {

		relation := object.Obj.(*topoAPI.Object_Relation)
		e2NodeID := relation.Relation.TgtEntityID
		e2NodeIDs = append(e2NodeIDs, e2NodeID)

	}

	return e2NodeIDs, nil
}

func getControlRelationFilter() *topoAPI.Filters {

	controlRelationFilter := &topoAPI.Filters{

		KindFilter: &topoAPI.Filter{

			Filter: &topoAPI.Filter_Equal_{

				Equal_: &topoAPI.EqualFilter{

					Value: topoAPI.CONTROLS,
				},
			},
		},
	}

	return controlRelationFilter

}

// WatchE2Connections watch e2 node connection changes
func (self *Client) WatchE2Connections(context context.Context, channel chan topoAPI.Event) error {

	err := self.client.Watch(context, channel, topoSDK.WithWatchFilters(getControlRelationFilter()))

	if err != nil {

		return err

	}

	return nil

}

func (c *Client) GetE2CellFilter() *topoAPI.Filters {
	cellEntityFilter := &topoAPI.Filters{
		KindFilter: &topoAPI.Filter{
			Filter: &topoAPI.Filter_In{
				In: &topoAPI.InFilter{
					Values: []string{topoAPI.E2CELL},
				},
			},
		},
	}
	return cellEntityFilter
}

func (c *Client) GetCellTypes(ctx context.Context) (map[string]Cell, error) {
	output := make(map[string]Cell)

	cells, err := c.client.List(ctx, topoSDK.WithListFilters(c.GetE2CellFilter()))
	if err != nil {
		log.Warn(err)
		return output, err
	}

	for _, cell := range cells {

		cellObject := &topoAPI.E2Cell{}
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
	cell, err := c.client.Get(ctx, topoAPI.ID(id))
	if err != nil {
		log.Warn(err)
		return err
	}

	cellObject := &topoAPI.E2Cell{}
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
