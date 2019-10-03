package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/shopspring/decimal"
)

const iPhone11Max = "iPhone 11 Pro Max"
const (
	// Si quieren buscar en otro lado reemplazaran MLA por su pais
	baseMeLiURL = "https://api.mercadolibre.com/sites/MLA/search"
	siteIDKey   = "site_id"
	queryKey    = "q"
	sortKey     = "sort"
	sortID      = "price_desc"
	resultsKey  = "results"
	priceKey    = "price"
)

// ResultadosML contiene un listado de resultados, representa una página de resultados.
type ResultadosML struct {
	Results []ResultadoML `json:"results"`
}

// ResultadoML contiene el precio de un resultado, representa un item de una página de resultados
// pero no es para nada exaustivo.
type ResultadoML struct {
	Price float64 `json:"price"`
}

// GetPrice devuelve el precio de un resultado convertido a decimal.Decimal.
func (r ResultadoML) GetPrice() decimal.Decimal {
	return decimal.NewFromFloat(r.Price)
}

func queryML() (io.ReadCloser, error) {
	queryURL, err := url.Parse(baseMeLiURL)
	if err != nil {
		return nil, fmt.Errorf("parsing mercado libre url: %v", err)
	}
	// Obtenemos un diccionario de clave/valor de los parametros de GET
	queryValues := queryURL.Query()
	// Agregamos los parametros que nos interesan
	// Ordenar por mas caro primero
	queryValues[sortKey] = []string{sortID}
	// Criterio de búsquda: un teléfono carísimo
	queryValues[queryKey] = []string{iPhone11Max}
	// Re-asignamos el diccionario de valores a la query original.
	queryURL.RawQuery = queryValues.Encode()
	// Realizamos la consulta.
	response, err := http.Get(queryURL.String())
	if err != nil {
		return nil, fmt.Errorf("querying mercado libre url: %v", err)
	}
	return response.Body, nil
}

func iPhoneMasCaroMLStruct(wg *sync.WaitGroup) (decimal.Decimal, error) {
	defer wg.Done()
	// Convertimos la URL a un objeto url.URL

	body, err := queryML()
	if err != nil {
		return decimal.Zero, err
	}
	defer body.Close()

	bodyData, err := ioutil.ReadAll(body)
	if err != nil {
		return decimal.Zero, fmt.Errorf("reading mercado libre response body: %v", err)
	}

	resultML := &ResultadosML{}
	err = json.Unmarshal(bodyData, &resultML)
	if err != nil {
		return decimal.Zero, fmt.Errorf("unmarshaling mercado libre response body: %v", err)
	}
	if len(resultML.Results) == 0 {
		return decimal.Zero, fmt.Errorf("results not found in response")
	}
	result := resultML.Results[0]

	return result.GetPrice(), nil
}

func iPhoneMasCaroML(wg *sync.WaitGroup) (decimal.Decimal, error) {
	defer wg.Done()
	body, err := queryML()
	if err != nil {
		return decimal.Zero, err
	}
	defer body.Close()

	bodyData, err := ioutil.ReadAll(body)
	if err != nil {
		return decimal.Zero, fmt.Errorf("reading mercado libre response body: %v", err)
	}

	resultML := map[string]interface{}{}
	err = json.Unmarshal(bodyData, &resultML)
	if err != nil {
		return decimal.Zero, fmt.Errorf("unmarshaling mercado libre response body: %v", err)
	}
	resultsRaw, ok := resultML[resultsKey]
	if !ok {
		return decimal.Zero, fmt.Errorf("key %s not found in response JSON", resultsKey)
	}
	results, ok := resultsRaw.([]interface{})
	if !ok {
		return decimal.Zero, fmt.Errorf("unexpected results type %T", resultsRaw)
	}
	if len(results) == 0 {
		return decimal.Zero, fmt.Errorf("nobody is selling an %s", iPhone11Max)
	}
	resultRaw := results[0]
	result, ok := resultRaw.(map[string]interface{})
	if !ok {
		return decimal.Zero, fmt.Errorf("unexpected single type %T", resultRaw)
	}
	priceRaw, ok := result[priceKey]
	if !ok {
		return decimal.Zero, fmt.Errorf("price is not available")
	}

	var moneyPrice decimal.Decimal
	switch price := priceRaw.(type) {
	case float64:
		moneyPrice = decimal.NewFromFloat(price)
	case float32:
		moneyPrice = decimal.NewFromFloat32(price)
	case string:
		moneyPrice, err = decimal.NewFromString(price)
		if err != nil {
			return decimal.Zero, fmt.Errorf("cannot translate price to a decimal value: %v", err)
		}
	default:
		return decimal.Zero, fmt.Errorf("price is not a type we can convert to decimal, is %T", priceRaw)
	}

	return moneyPrice, nil
}

func main() {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	// moneyPrice, err := iPhoneMasCaroML(wg)
	moneyPrice, err := iPhoneMasCaroMLStruct(wg)
	if err != nil {
		log.Fatalf("no se puede obtener el costo del iphone de mercado libre: %v", err)
	}
	usd, err := dolarizame(moneyPrice)
	if err != nil {
		fmt.Printf("el iphone mas caro cuesta: AR$ %s\n", moneyPrice.StringFixedBank(2))
		log.Fatalf("no se puede obtener la taza de cambio en dolares: %v", err)
	}
	fmt.Printf("el iphone mas caro cuesta: AR$ %s (U$D%s al promedio compra/venta)\n",
		moneyPrice.StringFixedBank(2), usd.StringFixedBank(2))
}
