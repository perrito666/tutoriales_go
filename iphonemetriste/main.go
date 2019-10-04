package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

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
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requesting to mercado libre: %s", response.Status)
	}
	return response.Body, nil
}

func iPhoneMasCaroMLStruct() (decimal.Decimal, error) {
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

func iPhoneMasCaroML() (decimal.Decimal, error) {
	// obtendremos el cuerpo de la respuesta de la función queryML, que es un io.ReadCloser
	body, err := queryML()
	if err != nil {
		return decimal.Zero, err
	}
	// recordaremos cerrar el cuerpo al finalizar
	defer body.Close()

	// leemos todo el cuerpo en un arreglo de bytes, no es recomendable hacerlo de esta manera
	// en código de producción ya que no estamos chequeando el largo del contenido antes de
	// guardarlo en memoria, si nuestro código hiciese muchas de estas llamadas posiblemente
	// ocuparia mucha memoria.
	bodyData, err := ioutil.ReadAll(body)
	if err != nil {
		return decimal.Zero, fmt.Errorf("reading mercado libre response body: %v", err)
	}

	// de-serializamos el contenido del cuerpo a un map[string]interface{}
	resultML := map[string]interface{}{}
	err = json.Unmarshal(bodyData, &resultML)
	if err != nil {
		return decimal.Zero, fmt.Errorf("unmarshaling mercado libre response body: %v", err)
	}

	// buscamos en el map, la clave de la lista de resultados
	resultsRaw, ok := resultML[resultsKey]
	if !ok {
		return decimal.Zero, fmt.Errorf("key %s not found in response JSON", resultsKey)
	}
	
	// convertimos de un objeto interface{} a un []interface para poder utilizar las
	// características de una lista
	results, ok := resultsRaw.([]interface{})
	if !ok {
		return decimal.Zero, fmt.Errorf("unexpected results type %T", resultsRaw)
	}
	
	// chequeamos que, ademas de ser una lita, tenga en efecto resultados.
	if len(results) == 0 {
		return decimal.Zero, fmt.Errorf("nobody is selling an %s", iPhone11Max)
	}

	// obtenemos el primer resultado que, dado el ordenamiento de mas caro a mas barato
	// debería ser el mas caro.
	resultRaw := results[0]

	// convertimos este resultado nuevamente a un tipo que podamos manipular.
	result, ok := resultRaw.(map[string]interface{})
	if !ok {
		return decimal.Zero, fmt.Errorf("unexpected single type %T", resultRaw)
	}

	// buscamos la clave del precio
	priceRaw, ok := result[priceKey]
	if !ok {
		return decimal.Zero, fmt.Errorf("price is not available")
	}


	// utilizamos type switch para convertir el precio a decimal desde varios tipos
	// posibles.
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

	// moneyPrice, err := iPhoneMasCaroML(wg)
	moneyPrice, err := iPhoneMasCaroMLStruct()
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
