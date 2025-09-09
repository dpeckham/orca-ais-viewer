import React, { useEffect, useRef, useState } from 'react';
import { StyleSheet, View } from 'react-native';
import Mapbox, { Camera, Images, MapView, SymbolLayer } from '@rnmapbox/maps';
import { Dimensions } from 'react-native';
import { ShapeSource } from '@rnmapbox/maps';
const _ = require('lodash');

//this key is unique to the Orca challenge and will be revoked. Otherwise, I'd store it securely elsewhere.
Mapbox.setAccessToken('pk.eyJ1IjoiZHBlY2toYW0iLCJhIjoiY21mOHR6ZTV6MTJhejJpb205dTdodTBrbSJ9.CTbBdHJlL0Zkr3A0QEd6Wg');

const EMPTY_SET = { "type": "FeatureCollection", "features": [] }
const NEWPORT_RI = [-71.310196, 41.490578];
const NEW_YORK = [-74,40.730610]
const START_LOC = NEW_YORK
const MIN_ZOOM = 12
const INITIAL_ZOOM = MIN_ZOOM

const App = () => {
  const mapViewRef = useRef<Mapbox.MapView>(null);
  const symbolLayerRef = useRef<SymbolLayer>(null);
  const [aisTargets, setAisTargets] = useState(EMPTY_SET)
  const [boundingBox, setBoundingBox] = useState([[-72, 44],[-68, 38]])

  const windowWidth = Dimensions.get('window').width;
  const windowHeight = Dimensions.get('window').height;

  const styles = StyleSheet.create({
    page: {
      flex: 1,
      justifyContent: 'center',
      alignItems: 'center',
    },
    container: {
      height: windowHeight,
      width: windowWidth,
    },
    map: {
      flex: 1
    }
  });

  function getVisibleBoundingBox() {
    mapViewRef.current?.getVisibleBounds().then((value) => {
      const newBounds = [[value[0][0],value[0][1]],[value[1][0],value[1][1]]]
      if (!_.isEqual(boundingBox,newBounds)) {
        setBoundingBox(newBounds)
      }
    })
  }

  useEffect(() => {
    const ws = new WebSocket('ws://localhost:8080/ais');

    ws.onopen = () => {
      const subscribeMessage = {
        "type": "subscribe",
        "boundingBox": boundingBox
      }
      console.log("Connecting with new subscription:", JSON.stringify(subscribeMessage))
      ws.send(JSON.stringify(subscribeMessage));
    };

    //todo: stop on zoom level
    ws.onmessage = e => {
      //todo: error check
      const message = JSON.parse(e.data)
      if (message['type'] == "FeatureCollection") {
        setAisTargets(message);
      }
    };

    ws.onerror = e => {
      console.log("Connection error: ", e.message);
    };

    ws.onclose = e => {
      console.log("Connection closed: ", e.code, e.reason);
    };

    return () => {
      ws.close()
    }
  }, [boundingBox]) //don't run again unless boundingBox changes.

  function onMapIdle() {
    getVisibleBoundingBox()
  }

  return (
    <View style={styles.page}>
      <View style={styles.container}>
        <MapView ref={mapViewRef} style={styles.map} onMapIdle={onMapIdle}>
          <Camera zoomLevel={INITIAL_ZOOM} centerCoordinate={START_LOC} />
          <Images images={{ ship: require('./assets/ship.png') }} />
          <ShapeSource id='ais' shape={aisTargets} />
          <SymbolLayer
            ref={symbolLayerRef}
            id='symbols' sourceID='ais' 
            minZoomLevel={MIN_ZOOM}
            style={{
              iconImage: 'ship',
              iconRotate: ['get', 'heading'],
              iconAllowOverlap: true,
              iconRotationAlignment: 'map'
            }}></SymbolLayer>
        </MapView>
      </View>
    </View>
  );
}

export default App;

