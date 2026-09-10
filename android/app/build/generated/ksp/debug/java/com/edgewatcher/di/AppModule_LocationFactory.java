package com.edgewatcher.di;

import android.content.Context;
import com.edgewatcher.domain.port.LocationGateway;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata("dagger.hilt.android.qualifiers.ApplicationContext")
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_LocationFactory implements Factory<LocationGateway> {
  private final Provider<Context> contextProvider;

  private AppModule_LocationFactory(Provider<Context> contextProvider) {
    this.contextProvider = contextProvider;
  }

  @Override
  public LocationGateway get() {
    return location(contextProvider.get());
  }

  public static AppModule_LocationFactory create(Provider<Context> contextProvider) {
    return new AppModule_LocationFactory(contextProvider);
  }

  public static LocationGateway location(Context context) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.location(context));
  }
}
