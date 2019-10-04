package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	"github.com/shopspring/decimal"
)

type mlSite struct {
	DefaultCurrencyID string `json:"default_currency_id"`
	ID                string `json:"id"`
	Name              string `json:"name"`
}

const mlSiteFetchEndpoint = "https://api.mercadolibre.com/sites"

func fetchSites() ([]mlSite, error) {
	response, err := http.Get(mlSiteFetchEndpoint)
	if err != nil {
		return nil, fmt.Errorf("querying mercado libre sites endpoint: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requesting to mercado libre sites list: %s", response.Status)
	}
	defer response.Body.Close()
	bodyData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading mercado libre sites list body: %v", err)
	}
	availableSites := []mlSite{}
	err = json.Unmarshal(bodyData, &availableSites)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling mercado libre sites list: %v", err)
	}

	return availableSites, nil
}

const (
	baseMeLiURL = "https://api.mercadolibre.com/sites/%s/search"
	queryKey    = "q"
	sortKey     = "sort"

	sortID = "price_desc"
)

func queryML(searchCriteria string, site mlSite) (io.ReadCloser, error) {
	queryURL, err := url.Parse(fmt.Sprintf(baseMeLiURL, site.ID))
	if err != nil {
		return nil, fmt.Errorf("parsing mercado libre url: %v", err)
	}
	// Obtenemos un diccionario de clave/valor de los parametros de GET
	queryValues := queryURL.Query()
	// Agregamos los parametros que nos interesan
	// Ordenar por mas caro primero
	queryValues[sortKey] = []string{sortID}
	// Criterio de búsquda: un teléfono carísimo
	queryValues[queryKey] = []string{searchCriteria}
	// Re-asignamos el diccionario de valores a la query original.
	queryURL.RawQuery = queryValues.Encode()
	// Realizamos la consulta.
	response, err := http.Get(queryURL.String())
	if err != nil {
		return nil, fmt.Errorf("querying mercado libre url: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requesting to mercado libre: %s", response.Status)
	}
	return response.Body, nil
}

// ResultadosML contiene un listado de resultados, representa una página de resultados.
type ResultadosML struct {
	Results []ResultadoML `json:"results"`
}

// ResultadoML contiene el precio de un resultado, representa un item de una página de resultados
// pero no es para nada exaustivo.
type ResultadoML struct {
	Price      float64 `json:"price"`
	Title      string  `json:"title"`
	Permalink  string  `json:"permalink"`
	CurrencyID string  `json:"currency_id"`
}

// GetPrice devuelve el precio de un resultado convertido a decimal.Decimal.
func (r ResultadoML) GetPrice() decimal.Decimal {
	return decimal.NewFromFloat(r.Price)
}

type siteSearchResult struct {
	site     mlSite
	price    decimal.Decimal
	priceUSD decimal.Decimal
	ratio    decimal.Decimal
	item     string
	err      error
}

const (
	meliCurrencyConversionURL = "https://api.mercadolibre.com/currency_conversions/search"
	meliCurrencyFrom          = "from"
	meliCurrencyTo            = "to"
)

const usdCurrencyCode = "USD"

type conversionRatio struct {
	Ratio float64 `json:"ratio"`
}

func (c conversionRatio) Decimal() decimal.Decimal {
	return decimal.NewFromFloat(c.Ratio)
}

func fetchCurrencyRate(sourceCurrency string) (decimal.Decimal, error) {
	meliURL, err := url.Parse(meliCurrencyConversionURL)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parsing mercado libre conversion api URL: %v", err)
	}
	queryValues := meliURL.Query()
	queryValues[meliCurrencyFrom] = []string{sourceCurrency}
	queryValues[meliCurrencyTo] = []string{usdCurrencyCode}
	meliURL.RawQuery = queryValues.Encode()

	response, err := http.Get(meliURL.String())
	if err != nil {
		return decimal.Zero, fmt.Errorf("querying mercado libre currency url: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		return decimal.Zero, fmt.Errorf("requesting currency to mercado libre: %s", response.Status)
	}

	bodyData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return decimal.Zero, fmt.Errorf("reading body from mercado libre currency url: %v", err)
	}

	ratio := &conversionRatio{}
	err = json.Unmarshal(bodyData, ratio)
	if err != nil {
		return decimal.Zero, fmt.Errorf("unmarshaling body from mercado libre currency url: %v", err)
	}

	return ratio.Decimal(), nil
}

func queryForSite(searchCriteria string, site mlSite,
	callerWaiting *sync.WaitGroup, result chan siteSearchResult) {
	defer callerWaiting.Done()

	currencyWait := &sync.WaitGroup{}
	currencyWait.Add(1)
	var currencyRatio decimal.Decimal
	var currencyError error

	go func() {
		defer currencyWait.Done()
		currencyRatio, currencyError = fetchCurrencyRate(site.DefaultCurrencyID)
	}()

	body, err := queryML(searchCriteria, site)
	if err != nil {
		result <- siteSearchResult{
			site: site,
			err:  err,
		}
		return
	}

	bodyData, err := ioutil.ReadAll(body)
	if err != nil {
		result <- siteSearchResult{
			site: site,
			err:  fmt.Errorf("reading mercado libre response body: %v", err),
		}
		return
	}

	resultML := &ResultadosML{}
	err = json.Unmarshal(bodyData, &resultML)
	if err != nil {
		result <- siteSearchResult{
			site: site,
			err:  fmt.Errorf("unmarshaling mercado libre response body: %v", err),
		}
		return
	}
	if len(resultML.Results) == 0 {
		result <- siteSearchResult{
			site: site,
			err:  fmt.Errorf("results not found in response"),
		}
		return
	}

	currencyWait.Wait()
	if currencyError != nil {
		result <- siteSearchResult{
			site: site,
			err:  fmt.Errorf("getting currency ratio: %v", currencyError),
		}
		return
	}

	//fmt.Println(site.Name)
	//fmt.Println(resultML.Results[0].Title)
	//fmt.Println(resultML.Results[0].Permalink)
	mlResult := resultML.Results[0]
	var price, priceUSD decimal.Decimal
	if mlResult.CurrencyID == usdCurrencyCode {
		priceUSD = mlResult.GetPrice()
		price = priceUSD.Div(currencyRatio)
	} else {
		price = mlResult.GetPrice()
		priceUSD = price.Mul(currencyRatio)
	}
	result <- siteSearchResult{
		site:     site,
		priceUSD: priceUSD,
		price:    price,
		item:     resultML.Results[0].Title,
		ratio:    currencyRatio,
	}
}
