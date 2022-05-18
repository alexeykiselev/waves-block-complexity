package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/wavesplatform/gowaves/pkg/client"
	"github.com/wavesplatform/gowaves/pkg/crypto"
	"github.com/wavesplatform/gowaves/pkg/proto"
)

const (
	defaultNetworkTimeout = 15 * time.Second
	defaultScheme         = "http"
)

type Complexity struct {
	ID              crypto.Digest `json:"id"`
	SpentComplexity int           `json:"spentComplexity"`
}

func main() {
	if err := run(); err != nil {
		switch err {
		case context.Canceled:
			os.Exit(130)
		default:
			os.Exit(1)
		}
	}
}

func run() error {
	var (
		node    string
		block   string
		timeout time.Duration
	)

	flag.StringVar(&node, "node", "nodes.wavesnodes.com", "Waves node API URL, default value is nodes.wavesnodes.com")
	flag.StringVar(&block, "block", "", "Block ID, no default value")
	flag.DurationVar(&timeout, "timeout", defaultNetworkTimeout, "Network timeout, seconds. Default value is 15")
	flag.Parse()

	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt)
	defer done()

	n, err := validateNodeURL(node)
	if err != nil {
		log.Printf("Invalid node URL '%s': %v", node, err)
		return err
	}
	cl := newClient(n, timeout)
	b, err := getBlock(ctx, cl, block)
	if err != nil {
		log.Printf("Failed to get block with ID '%s': %v", block, err)
		return err
	}
	scheme := b.Generator.Bytes()[1]
	complexities, err := getTransactionsComplexities(ctx, cl, *b, scheme)
	if err != nil {
		log.Printf("Failed to get transactions complexities: %v", err)
		return err
	}
	total := 0
	for _, c := range complexities {
		total += c.SpentComplexity
		if c.SpentComplexity > 0 {
			log.Printf("[%s]\t%d", c.ID.String(), c.SpentComplexity)
		}
	}
	log.Println()
	log.Printf("Block Complexity: %d", total)
	return nil
}

func getBlock(ctx context.Context, client *client.Client, id string) (*client.Block, error) {
	blockID, err := proto.NewBlockIDFromBase58(id)
	if err != nil {
		return nil, err
	}
	block, _, err := client.Blocks.Signature(ctx, blockID)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func getTransactionsComplexities(ctx context.Context, cl *client.Client, block client.Block, scheme byte) ([]Complexity, error) {
	r := make([]Complexity, 0, block.TransactionCount)
	for _, tx := range block.Transactions {
		d, err := tx.GetID(scheme)
		if err != nil {
			return nil, err
		}
		id, err := crypto.NewDigestFromBytes(d)
		if err != nil {
			return nil, err
		}
		c, err := getComplexity(ctx, cl, id)
		if err != nil {
			return nil, err
		}
		r = append(r, *c)
	}
	return r, nil
}

func getComplexity(ctx context.Context, cl *client.Client, id crypto.Digest) (*Complexity, error) {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/transactions/info/%s", cl.GetOptions().BaseUrl, id.String()), nil)
	if err != nil {
		return nil, err
	}
	res := new(Complexity)
	_, err = cl.Do(ctx, req, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func validateNodeURL(s string) (string, error) {
	var u *url.URL
	var err error
	if strings.Contains(s, "//") {
		u, err = url.Parse(s)
	} else {
		u, err = url.Parse("//" + s)
	}
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		u.Scheme = defaultScheme
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.Errorf("unsupported URL scheme '%s'", u.Scheme)
	}
	return u.String(), nil
}

func newClient(url string, timeout time.Duration) *client.Client {
	opts := client.Options{
		BaseUrl: url,
		Client:  &http.Client{Timeout: timeout},
	}
	// The error can be safely ignored because `NewClient` function only checks the number of passed `opts`
	cl, _ := client.NewClient(opts)
	return cl
}
