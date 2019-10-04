package main

import (
	"fmt"
	"log"
	"sync"
)

const iPhone11Max = "iPhone 11 Pro Max"

func main() {
	sites, err := fetchSites()
	if err != nil {
		log.Fatalf("could not obtain mercado libre sites: %v", err)
	}

	results := make([]siteSearchResult, 0, len(sites))
	wg := &sync.WaitGroup{}
	wg.Add(len(sites))

	resultChannel := make(chan siteSearchResult)
	for i := range sites {
		go queryForSite(iPhone11Max, sites[i], wg, resultChannel)
	}

	waitResultFetch := &sync.WaitGroup{}
	waitResultFetch.Add(1)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case r := <-resultChannel:
				if r.err != nil {
					fmt.Printf("Site %q failed %v\n", r.site.Name, r.err)
					break
				}
				results = append(results, r)
			case <-done:
				waitResultFetch.Done()
				return
			}
		}
	}()
	wg.Wait()
	close(done)
	waitResultFetch.Wait()
	for _, v := range results {
		fmt.Printf("Comprar un iPhone 11 Max Pro 512G en %q cuesta USD %s (son %s %s a cambio %s):\n",
			v.site.Name, v.priceUSD.StringFixedBank(2), v.site.DefaultCurrencyID, v.price.StringFixedBank(2), v.ratio)
		// fmt.Printf("--> Publicado como %q\n", v.item)
	}
}
