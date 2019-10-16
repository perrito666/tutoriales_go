package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

const iPhone11Max = "iPhone 11 Pro Max"

func main() {
	// Obtenemos de los argumentos de linea de comandos el criterio de búsqueda.
	searchTerms := iPhone11Max
	if len(os.Args) > 1 {
		searchTerms = strings.Join(os.Args[1:], " ")
	}
	// obtenemos de mercado libre los sitios internacionales
	sites, err := fetchSites()
	if err != nil {
		log.Fatalf("could not obtain mercado libre sites: %v", err)
	}

	// Hacemos una lista que contendrá los resultados de las búsquedas.
	results := make([]siteSearchResult, 0, len(sites))

	// creamos los WaitGroups para cada una de las go-rutinas que buscará.
	wg := &sync.WaitGroup{}
	wg.Add(len(sites))

	// creamos un canal, sin buffer, para los resultados.
	resultChannel := make(chan siteSearchResult)

	// instanciamos una gorutina por cada sitio de Mercado Libre
	for i := range sites {
		go queryForSite(searchTerms, sites[i], wg, resultChannel)
	}

	// creamos un WaitGroup para esperar la gorutina que procesa los resultados.
	waitResultFetch := &sync.WaitGroup{}
	waitResultFetch.Add(1)

	// Hacemos un contexto cancelable para indicar cuando estemos listos
	// para salir de la función de procesamiento de resultados.
	ctx, done := context.WithCancel(context.Background())

	// invocamos la función anónima de procesamiento de resultados pasando
	// el contexto como parámetro, notar el shadowing.
	go func(ctx context.Context) {
		for {
			select {
			case r := <-resultChannel:
				if r.err != nil {
					fmt.Printf("Site %q failed %v\n", r.site.Name, r.err)
					break
				}
				results = append(results, r)
			case <-ctx.Done():
				waitResultFetch.Done()
				return
			}
		}
	}(ctx)

	// esperamos el wait group de todas las gorutinas de búsqueda, que no terminarán hasta
	// que la funcion de procesamiento haya leido su resultado.
	wg.Wait()

	// indicamos a la función de procesamiento que ya no queda nada por procesar
	done()

	// esperamos que la función de procesamiento termine.
	waitResultFetch.Wait()

	// imprimimos los resultados
	for _, v := range results {
		fmt.Printf("Comprar %q en %q cuesta USD %s (son %s %s a cambio %s):\n",
			searchTerms, v.site.Name, v.priceUSD.StringFixedBank(2), v.site.DefaultCurrencyID, v.price.StringFixedBank(2), v.ratio)
		fmt.Printf("--> Publicado como %q\n", v.item)
	}
}
